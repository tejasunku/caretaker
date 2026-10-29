// Package lineage records patch parentage as OCI referrer artifacts. The
// artifacts are attached to the patched child and to its parent, so lineage
// survives even when image tags are deleted. Nothing here mutates the image
// manifest or its digest.
package lineage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/registry"
)

// Annotation keys used on lineage artifacts.
const (
	AnnotationKind         = "caretaker.io/lineage"
	AnnotationService      = "caretaker.io/service"
	AnnotationBaseDigest   = "caretaker.io/base-digest"
	AnnotationBaseVersion  = "caretaker.io/base-version"
	AnnotationSourceDigest = "caretaker.io/source-digest"
	AnnotationParentDigest = "caretaker.io/parent-digest"
	AnnotationChildDigest  = "caretaker.io/child-digest"
	AnnotationPatchedAt    = "caretaker.io/patched-at"

	KindMetadata = "metadata"
	KindChild    = "child"
)

// Metadata describes how one patched image was produced.
type Metadata struct {
	Service      string `json:"service,omitempty"`
	BaseDigest   string `json:"baseDigest,omitempty"`
	BaseVersion  string `json:"baseVersion,omitempty"`
	SourceDigest string `json:"sourceDigest,omitempty"`
	ParentDigest string `json:"parentDigest,omitempty"`
	PatchedAt    string `json:"patchedAt,omitempty"`
}

// Child is a patched image descended from a parent.
type Child struct {
	Digest      string
	Service     string
	BaseDigest  string
	BaseVersion string
	PatchedAt   string
}

// Recorder writes lineage artifacts.
type Recorder interface {
	Record(ctx context.Context, childRef string, meta Metadata) error
}

// Reader discovers lineage.
type Reader interface {
	Metadata(ctx context.Context, childRef string) (Metadata, bool, error)
	Children(ctx context.Context, parentRef string) ([]Child, error)
}

// Recorder stores lineage in the OCI registry.
type RecorderClient struct {
	Registry *registry.Client
}

var _ Recorder = (*RecorderClient)(nil)
var _ Reader = (*RecorderClient)(nil)

// Record attaches metadata to the patched child and a child pointer to the
// parent. Both subjects are addressed by digest.
func (r *RecorderClient) Record(ctx context.Context, childRef string, meta Metadata) error {
	child, err := image.Parse(childRef)
	if err != nil {
		return fmt.Errorf("parse child %s: %w", childRef, err)
	}
	if child.Digest == "" {
		return fmt.Errorf("child %s has no digest", childRef)
	}
	if meta.PatchedAt == "" {
		meta.PatchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if meta.ParentDigest == "" {
		meta.ParentDigest = meta.SourceDigest
	}

	childAnnotations := map[string]string{
		AnnotationKind:         KindMetadata,
		AnnotationChildDigest:  child.Digest,
		AnnotationService:      meta.Service,
		AnnotationBaseDigest:   meta.BaseDigest,
		AnnotationBaseVersion:  meta.BaseVersion,
		AnnotationSourceDigest: meta.SourceDigest,
		AnnotationParentDigest: meta.ParentDigest,
		AnnotationPatchedAt:    meta.PatchedAt,
	}
	if _, err := r.Registry.PushArtifact(ctx, child.Repository+"@"+child.Digest, childAnnotations); err != nil {
		return fmt.Errorf("record lineage on %s: %w", childRef, err)
	}

	parentDigest := meta.BaseDigest
	if parentDigest == "" {
		parentDigest = meta.ParentDigest
	}
	if parentDigest == "" {
		return nil
	}
	parentRef := child.Repository + "@" + parentDigest
	parentAnnotations := map[string]string{
		AnnotationKind:        KindChild,
		AnnotationChildDigest: child.Digest,
		AnnotationService:     meta.Service,
		AnnotationBaseDigest:  parentDigest,
		AnnotationBaseVersion: meta.BaseVersion,
		AnnotationPatchedAt:   meta.PatchedAt,
	}
	if _, err := r.Registry.PushArtifact(ctx, parentRef, parentAnnotations); err != nil {
		return fmt.Errorf("record child pointer on %s: %w", parentRef, err)
	}
	return nil
}

// Metadata returns the lineage recorded on a patched child.
func (r *RecorderClient) Metadata(ctx context.Context, childRef string) (Metadata, bool, error) {
	arts, err := r.Registry.Referrers(ctx, childRef)
	if err != nil {
		return Metadata{}, false, err
	}
	for _, art := range arts {
		ann, err := r.annotations(ctx, childRef, art)
		if err != nil {
			continue
		}
		if ann[AnnotationKind] != KindMetadata {
			continue
		}
		return Metadata{
			Service:      ann[AnnotationService],
			BaseDigest:   ann[AnnotationBaseDigest],
			BaseVersion:  ann[AnnotationBaseVersion],
			SourceDigest: ann[AnnotationSourceDigest],
			ParentDigest: ann[AnnotationParentDigest],
			PatchedAt:    ann[AnnotationPatchedAt],
		}, true, nil
	}
	return Metadata{}, false, nil
}

// Children returns patched images recorded under a parent digest, oldest first.
func (r *RecorderClient) Children(ctx context.Context, parentRef string) ([]Child, error) {
	arts, err := r.Registry.Referrers(ctx, parentRef)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]Child, 0, len(arts))
	for _, art := range arts {
		ann, err := r.annotations(ctx, parentRef, art)
		if err != nil {
			continue
		}
		if ann[AnnotationKind] != KindChild || ann[AnnotationChildDigest] == "" {
			continue
		}
		if seen[ann[AnnotationChildDigest]] {
			continue
		}
		seen[ann[AnnotationChildDigest]] = true
		out = append(out, Child{
			Digest:      ann[AnnotationChildDigest],
			Service:     ann[AnnotationService],
			BaseDigest:  ann[AnnotationBaseDigest],
			BaseVersion: ann[AnnotationBaseVersion],
			PatchedAt:   ann[AnnotationPatchedAt],
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].PatchedAt < out[j].PatchedAt
	})
	return out, nil
}

func (r *RecorderClient) annotations(ctx context.Context, subjectRef string, art registry.Artifact) (map[string]string, error) {
	if len(art.Annotations) > 0 {
		return art.Annotations, nil
	}
	ref, err := image.Parse(subjectRef)
	if err != nil {
		return nil, err
	}
	return r.Registry.ArtifactAnnotations(ctx, ref.Repository+"@"+art.Digest)
}

// OldestChild returns the first child by patch time.
func OldestChild(children []Child) (Child, bool) {
	if len(children) == 0 {
		return Child{}, false
	}
	return children[0], true
}

// NewestChild returns the last child by patch time.
func NewestChild(children []Child) (Child, bool) {
	if len(children) == 0 {
		return Child{}, false
	}
	return children[len(children)-1], true
}

// ParsePolicy validates a parent selection policy.
func ParsePolicy(policy string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(policy))
	switch p {
	case "", "prefer-parent", "oldest-child", "newest-child", "none":
		if p == "" {
			return "prefer-parent", nil
		}
		return p, nil
	default:
		return "", fmt.Errorf("unknown parent policy %q (want prefer-parent, oldest-child, newest-child, or none)", policy)
	}
}
