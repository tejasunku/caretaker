// Package webhook implements a mutating admission webhook that rewrites
// Deployment images from the preference map. It runs on the workstation; the
// API server calls it over HTTPS.
package webhook

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/kube"
)

// Resolver resolves an image reference to a manifest digest.
type Resolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// Mutator rewrites Deployment container images using the preference map.
type Mutator struct {
	Registry        Resolver
	LoadPreferences func() (*catalog.File, error)
	AllowTags       bool
}

type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// Mutate returns JSONPatch operations for a Deployment. An empty slice means
// the object is already correct or no preference applies.
func (m *Mutator) Mutate(ctx context.Context, d *appsv1.Deployment) ([]patchOp, error) {
	if d == nil || m.Registry == nil || m.LoadPreferences == nil {
		return nil, nil
	}
	if d.Annotations[kube.AnnotationSkipWebhook] == "true" {
		return nil, nil
	}
	prefs, err := m.LoadPreferences()
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		return nil, nil
	}

	var ops []patchOp
	annotations := make(map[string]string, len(d.Annotations)+4)
	for k, v := range d.Annotations {
		annotations[k] = v
	}
	annotationsChanged := false

	for i := range d.Spec.Template.Spec.Containers {
		container := &d.Spec.Template.Spec.Containers[i]
		ref, err := image.Parse(container.Image)
		if err != nil {
			continue
		}

		target, service, ok := m.preferred(ctx, container.Name, ref, d.Annotations, prefs)
		if !ok && d.Name != container.Name {
			target, service, ok = m.preferred(ctx, d.Name, ref, d.Annotations, prefs)
		}
		if !ok {
			continue
		}
		targetRef, err := image.Parse(target)
		if err != nil {
			continue
		}
		if !ref.SameImage(targetRef) {
			ops = append(ops, patchOp{
				Op:    "replace",
				Path:  fmt.Sprintf("/spec/template/spec/containers/%d/image", i),
				Value: target,
			})
		}

		base := ref.Digest
		if v := d.Annotations[kube.AnnotationBaseDigest]; v != "" {
			base = v
		} else if ref.Digest != "" {
			if source, ok := prefs.SourceForTarget(service, ref.Repository+"@"+ref.Digest); ok {
				if parsed, err := image.Parse(source); err == nil && parsed.Digest != "" {
					base = parsed.Digest
				}
			}
		}
		if m.AllowTags && d.Annotations[kube.AnnotationBaseDigest] == "" {
			if version := baseVersion(ref, d.Annotations, true); version != "" {
				if digest, err := m.Registry.Resolve(ctx, ref.Repository+":"+version); err == nil {
					base = digest
				}
			}
		}
		if base != "" && annotations[kube.AnnotationBaseDigest] != base {
			annotations[kube.AnnotationBaseDigest] = base
			annotationsChanged = true
		}
		if version := baseVersion(ref, d.Annotations, m.AllowTags); version != "" && annotations[kube.AnnotationBaseVersion] != version {
			annotations[kube.AnnotationBaseVersion] = version
			annotationsChanged = true
		}
		if annotations[kube.AnnotationManaged] != "true" {
			annotations[kube.AnnotationManaged] = "true"
			annotationsChanged = true
		}
		if annotations[kube.AnnotationWebhook] != "true" {
			annotations[kube.AnnotationWebhook] = "true"
			annotationsChanged = true
		}
	}

	if annotationsChanged {
		op := "replace"
		if d.Annotations == nil {
			op = "add"
		}
		ops = append(ops, patchOp{Op: op, Path: "/metadata/annotations", Value: annotations})
	}
	return ops, nil
}

// preferred looks up a target by the running digest, then by the mapping that
// produced the running image, then by the recorded base digest. Tag matching
// is opt-in.
func (m *Mutator) preferred(ctx context.Context, service string, ref image.Ref, annotations map[string]string, prefs *catalog.File) (string, string, bool) {
	if ref.Digest != "" {
		running := ref.Repository + "@" + ref.Digest
		if target, ok := prefs.Lookup(service, running); ok {
			return target, service, true
		}
		if source, ok := prefs.SourceForTarget(service, running); ok {
			if target, ok := prefs.Lookup(service, source); ok {
				return target, service, true
			}
		}
	}
	if digest := annotations[kube.AnnotationBaseDigest]; digest != "" {
		if target, ok := prefs.Lookup(service, ref.Repository+"@"+digest); ok {
			return target, service, true
		}
	}
	if m.AllowTags {
		if version := baseVersion(ref, annotations, true); version != "" {
			digest, err := m.Registry.Resolve(ctx, ref.Repository+":"+version)
			if err == nil {
				if target, ok := prefs.Lookup(service, ref.Repository+"@"+digest); ok {
					return target, service, true
				}
			}
		}
	}
	return "", "", false
}

func baseVersion(ref image.Ref, annotations map[string]string, allowTags bool) string {
	if v := annotations[kube.AnnotationBaseVersion]; v != "" {
		return v
	}
	if !allowTags || ref.Tag == "" {
		return ""
	}
	v, _ := image.BaseVersion(ref.Tag)
	return v
}
