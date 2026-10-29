// Package lineage records patch parentage as OCI referrer artifacts. The
// artifacts are attached to the patched child and to its parent, so lineage
// survives even when image tags are deleted. Nothing here mutates the image
// manifest or its digest.
package lineage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/registry"
)

// Annotation keys used on lineage artifacts.
const (
	AnnotationKind            = "caretaker.io/lineage"
	AnnotationService         = "caretaker.io/service"
	AnnotationBaseDigest      = "caretaker.io/base-digest"
	AnnotationBaseVersion     = "caretaker.io/base-version"
	AnnotationSourceDigest    = "caretaker.io/source-digest"
	AnnotationParentDigest    = "caretaker.io/parent-digest"
	AnnotationChildDigest     = "caretaker.io/child-digest"
	AnnotationChildRepository = "caretaker.io/child-repository"
	AnnotationPatchedAt       = "caretaker.io/patched-at"

	KindMetadata = "metadata"
	KindChild    = "child"
	KindIndex    = "index"
)

// Artifact types caretaker publishes. artifactType is the only field the
// Referrers API can filter on, so each record kind gets its own media type.
const (
	// MediaTypeIndex anchors the per-repository lineage log.
	MediaTypeIndex = "application/vnd.caretaker.lineage.index.v1+json"
	// MediaTypeEntry is one append-only log line of the lineage log.
	MediaTypeEntry = "application/vnd.caretaker.lineage.v1+json"
	// MediaTypeMetadata is the fast-path record on a patched child.
	MediaTypeMetadata = "application/vnd.caretaker.lineage.metadata.v1+json"
	// MediaTypeChild is the child pointer recorded on a parent.
	MediaTypeChild = "application/vnd.caretaker.lineage.child.v1+json"
)

// IndexTag is the tag of the immutable lineage index manifest in every image
// repository. It is the subject anchor for the lineage log, so it stays tagged
// (and therefore live) even after every base image in the repo is deleted.
const IndexTag = "caretaker-lineage-index"

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

// Entry is one line of the repository's lineage log.
type Entry struct {
	Artifact        string // digest of the entry artifact itself
	Repository      string // repository that owns the log
	ChildRepository string
	ChildDigest     string
	ParentDigest    string
	SourceDigest    string
	BaseDigest      string
	BaseVersion     string
	Service         string
	PatchedAt       string
}

// Recorder writes lineage artifacts.
type Recorder interface {
	Record(ctx context.Context, childRef string, meta Metadata) error
}

// Reader discovers lineage.
type Reader interface {
	Metadata(ctx context.Context, childRef string) (Metadata, bool, error)
	Children(ctx context.Context, parentRef string) ([]Child, error)
	ChildrenForBase(ctx context.Context, repository, baseDigest string) ([]Child, error)
}

// Store is the registry surface lineage needs. *registry.Client implements it.
type Store interface {
	Exists(ctx context.Context, ref string) (bool, error)
	Resolve(ctx context.Context, ref string) (string, error)
	PushArtifact(ctx context.Context, subjectRef, artifactType string, annotations map[string]string) (string, error)
	EnsureArtifactTag(ctx context.Context, repository, tag, artifactType string, annotations map[string]string) (string, bool, error)
	Referrers(ctx context.Context, subjectRef string) ([]registry.Artifact, error)
	ReferrersOfType(ctx context.Context, subjectRef, artifactType string) ([]registry.Artifact, error)
	ArtifactAnnotations(ctx context.Context, ref string) (map[string]string, error)
}

// ErrNoIndex reports that a repository has no lineage index yet. Reads never
// create one; only a recorded patch does.
var ErrNoIndex = errors.New("lineage index not found")

// Recorder stores lineage in the OCI registry.
type RecorderClient struct {
	Registry Store
}

var _ Recorder = (*RecorderClient)(nil)
var _ Reader = (*RecorderClient)(nil)

// Record attaches metadata to the patched child and a child pointer to the
// immediate parent. Both subjects are addressed by digest.
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
		AnnotationKind:            KindMetadata,
		AnnotationChildDigest:     child.Digest,
		AnnotationChildRepository: child.Repository,
		AnnotationService:         meta.Service,
		AnnotationBaseDigest:      meta.BaseDigest,
		AnnotationBaseVersion:     meta.BaseVersion,
		AnnotationSourceDigest:    meta.SourceDigest,
		AnnotationParentDigest:    meta.ParentDigest,
		AnnotationPatchedAt:       meta.PatchedAt,
	}
	if _, err := r.Registry.PushArtifact(ctx, child.Repository+"@"+child.Digest, MediaTypeMetadata, childAnnotations); err != nil {
		return fmt.Errorf("record lineage on %s: %w", childRef, err)
	}

	// The parent is the immediate image we patched. The base is the original
	// image at the root of the patch chain. They are the same only when the
	// source was unpatched.
	parentDigest := meta.ParentDigest
	if parentDigest == "" {
		parentDigest = meta.BaseDigest
	}
	if parentDigest != "" {
		parentRef := child.Repository + "@" + parentDigest
		parentAnnotations := map[string]string{
			AnnotationKind:            KindChild,
			AnnotationChildDigest:     child.Digest,
			AnnotationChildRepository: child.Repository,
			AnnotationService:         meta.Service,
			AnnotationBaseDigest:      meta.BaseDigest,
			AnnotationBaseVersion:     meta.BaseVersion,
			AnnotationSourceDigest:    meta.SourceDigest,
			AnnotationParentDigest:    meta.ParentDigest,
			AnnotationPatchedAt:       meta.PatchedAt,
		}
		if _, err := r.Registry.PushArtifact(ctx, parentRef, MediaTypeChild, parentAnnotations); err != nil {
			return fmt.Errorf("record child pointer on %s: %w", parentRef, err)
		}
	}

	// The log is the durable copy: parent-side referrers die with the parent,
	// but the index manifest is tagged, so its referrers outlive every image
	// in the repository.
	if err := r.append(ctx, child.Repository, childAnnotations); err != nil {
		return err
	}
	return nil
}

// Index returns the digest of the repository's lineage index, creating it when
// absent. created reports whether this call made it.
func (r *RecorderClient) Index(ctx context.Context, repository string) (string, bool, error) {
	return r.Registry.EnsureArtifactTag(ctx, repository, IndexTag, MediaTypeIndex, map[string]string{
		AnnotationKind: KindIndex,
	})
}

// index resolves the repository's lineage index without creating it.
func (r *RecorderClient) index(ctx context.Context, repository string) (string, error) {
	ref := repository + ":" + IndexTag
	exists, err := r.Registry.Exists(ctx, ref)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%s: %w", ref, ErrNoIndex)
	}
	return r.Registry.Resolve(ctx, ref)
}

// append writes one line to the repository's lineage log.
func (r *RecorderClient) append(ctx context.Context, repository string, ann map[string]string) error {
	indexDigest, _, err := r.Index(ctx, repository)
	if err != nil {
		return fmt.Errorf("lineage index for %s: %w", repository, err)
	}
	entry := make(map[string]string, len(ann))
	for k, v := range ann {
		entry[k] = v
	}
	entry[AnnotationKind] = KindMetadata
	if _, err := r.Registry.PushArtifact(ctx, repository+"@"+indexDigest, MediaTypeEntry, entry); err != nil {
		return fmt.Errorf("append lineage log for %s: %w", repository, err)
	}
	return nil
}

// Log returns every lineage entry recorded against a repository's index, oldest
// first. Unlike Children, it does not depend on the parent image still
// existing. A repository with no index yet simply has an empty log.
func (r *RecorderClient) Log(ctx context.Context, repository string) ([]Entry, error) {
	indexDigest, err := r.index(ctx, repository)
	if err != nil {
		if errors.Is(err, ErrNoIndex) {
			return nil, nil
		}
		return nil, fmt.Errorf("lineage index for %s: %w", repository, err)
	}
	arts, err := r.Registry.ReferrersOfType(ctx, repository+"@"+indexDigest, MediaTypeEntry)
	if err != nil {
		return nil, fmt.Errorf("lineage log for %s: %w", repository, err)
	}
	out := make([]Entry, 0, len(arts))
	for _, art := range arts {
		ann, err := r.annotations(ctx, repository+"@"+indexDigest, art)
		if err != nil {
			continue
		}
		if ann[AnnotationKind] != KindMetadata || ann[AnnotationChildDigest] == "" {
			continue
		}
		out = append(out, Entry{
			Artifact:        art.Digest,
			Repository:      repository,
			ChildRepository: firstNonEmpty(ann[AnnotationChildRepository], repository),
			ChildDigest:     ann[AnnotationChildDigest],
			ParentDigest:    ann[AnnotationParentDigest],
			SourceDigest:    ann[AnnotationSourceDigest],
			BaseDigest:      ann[AnnotationBaseDigest],
			BaseVersion:     ann[AnnotationBaseVersion],
			Service:         ann[AnnotationService],
			PatchedAt:       ann[AnnotationPatchedAt],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PatchedAt < out[j].PatchedAt })
	return out, nil
}

// ChildrenForBase returns patched images recorded for a base digest in a
// repository's log, oldest first. It works after the base image is deleted.
func (r *RecorderClient) ChildrenForBase(ctx context.Context, repository, baseDigest string) ([]Child, error) {
	if baseDigest == "" {
		return nil, nil
	}
	entries, err := r.Log(ctx, repository)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]Child, 0, len(entries))
	for _, e := range entries {
		if !same(e.BaseDigest, baseDigest) || seen[e.ChildDigest] {
			continue
		}
		seen[e.ChildDigest] = true
		out = append(out, Child{
			Digest:      e.ChildDigest,
			Service:     e.Service,
			BaseDigest:  e.BaseDigest,
			BaseVersion: e.BaseVersion,
			PatchedAt:   e.PatchedAt,
		})
	}
	return out, nil
}

func same(a, b string) bool {
	return a != "" && a == b
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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

// BaseFor returns the root base digest and version recorded for ref. ok is
// false when ref has no lineage metadata, for example an unpatched base image.
func BaseFor(ctx context.Context, r Reader, ref string) (baseDigest, baseVersion string, ok bool, err error) {
	meta, ok, err := r.Metadata(ctx, ref)
	if err != nil || !ok {
		return "", "", false, err
	}
	if meta.BaseDigest == "" {
		return "", "", false, nil
	}
	return meta.BaseDigest, meta.BaseVersion, true, nil
}
