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

		// Recognize the image by its digest, whatever spelling the deployer
		// used. A tag is resolved to a digest first so a manifest that pins
		// "repo:1.0" is treated the same as "repo@sha256:...". Recognition is
		// keyed by digest across every service, not by the container name.
		digest := ref.Digest
		if digest == "" && ref.Tag != "" {
			if resolved, err := m.Registry.Resolve(ctx, container.Image); err == nil {
				digest = resolved
			}
		}
		pinned := ref
		if digest != "" {
			pinned = image.Ref{Repository: ref.Repository, Tag: ref.Tag, Digest: digest}
		}

		target, base, ok := m.recognize(ctx, pinned, d.Annotations, prefs)
		if !ok {
			continue
		}
		targetRef, err := image.Parse(target)
		if err != nil {
			continue
		}
		if !pinned.SameImage(targetRef) {
			ops = append(ops, patchOp{
				Op:    "replace",
				Path:  fmt.Sprintf("/spec/template/spec/containers/%d/image", i),
				Value: target,
			})
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

// recognize finds the preferred target for a container image, identifying the
// image by its digest across every service (independent of the container or
// Deployment name). It returns the target and the feature-build (base) digest
// to record. Tag fallbacks are opt-in and only used once digest recognition
// has failed.
func (m *Mutator) recognize(ctx context.Context, ref image.Ref, annotations map[string]string, prefs *catalog.File) (target, base string, ok bool) {
	if ref.Digest != "" {
		pinned := ref.Repository + "@" + ref.Digest
		// The image is a feature build (or one of its aliases): prefer its
		// patched target, recording the canonical feature-build digest.
		if _, m, found := prefs.MappingAnyService(pinned); found {
			return m.Target, bareDigest(m.Base), true
		}
		// The image is a patched target we already prefer: keep it and record
		// the feature build it came from.
		if _, b, found := prefs.BaseForTargetAnyService(pinned); found {
			if _, t, found := prefs.LookupAnyService(b); found {
				return t, bareDigest(b), true
			}
			return pinned, bareDigest(b), true
		}
	}
	// The Deployment declares its base via annotation.
	if b := annotations[kube.AnnotationBaseDigest]; b != "" {
		if _, t, found := prefs.LookupAnyService(ref.Repository + "@" + b); found {
			return t, b, true
		}
	}
	// Legacy opt-in: a patched tag encodes the base version (1.0-patched-…).
	if m.AllowTags {
		if version := baseVersion(ref, annotations, true); version != "" {
			if digest, err := m.Registry.Resolve(ctx, ref.Repository+":"+version); err == nil {
				if _, t, found := prefs.LookupAnyService(ref.Repository + "@" + digest); found {
					return t, digest, true
				}
			}
		}
	}
	return "", "", false
}

// bareDigest reduces a repository reference to its digest ("sha256:…").
func bareDigest(ref string) string {
	if r, err := image.Parse(ref); err == nil {
		return r.Digest
	}
	return ""
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
