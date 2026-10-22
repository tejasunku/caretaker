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
	annotations := make(map[string]string, len(d.Annotations)+3)
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

		target, ok := m.preferred(ctx, container.Name, ref, d.Annotations, prefs)
		if !ok && d.Name != container.Name {
			target, ok = m.preferred(ctx, d.Name, ref, d.Annotations, prefs)
		}
		if !ok {
			continue
		}
		targetRef, err := image.Parse(target)
		if err != nil || ref.SameImage(targetRef) {
			continue
		}

		ops = append(ops, patchOp{
			Op:    "replace",
			Path:  fmt.Sprintf("/spec/template/spec/containers/%d/image", i),
			Value: target,
		})

		if version := baseVersion(ref, d.Annotations); version != "" && annotations[kube.AnnotationBaseVersion] != version {
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

// preferred looks up a target by the running digest first, then by the digest
// of the base version. The base version comes from the annotation or a
// -patched- tag; it is resolved through the registry.
func (m *Mutator) preferred(ctx context.Context, service string, ref image.Ref, annotations map[string]string, prefs *catalog.File) (string, bool) {
	if ref.Digest != "" {
		if target, ok := prefs.Lookup(service, ref.Repository+"@"+ref.Digest); ok {
			return target, true
		}
	}
	version := baseVersion(ref, annotations)
	if version == "" {
		return "", false
	}
	digest, err := m.Registry.Resolve(ctx, ref.Repository+":"+version)
	if err != nil {
		return "", false
	}
	return prefs.Lookup(service, ref.Repository+"@"+digest)
}

func baseVersion(ref image.Ref, annotations map[string]string) string {
	if v := annotations[kube.AnnotationBaseVersion]; v != "" {
		return v
	}
	if ref.Tag == "" {
		return ""
	}
	v, _ := image.BaseVersion(ref.Tag)
	return v
}
