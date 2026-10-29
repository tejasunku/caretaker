package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/config"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/registry"
)

const (
	sourceRepo        = "localhost:5000/test/api"
	sourceBaseDigest  = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sourceOldChild    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	sourceNewChild    = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	sourceIndexDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

func sourceApp() *App {
	return &App{cfg: config.Default(), AllowTags: true, Out: io.Discard, Err: io.Discard}
}

// indexEntry builds one lineage log entry as it would come back from the
// referrers list of the repository's lineage index.
func indexEntry(childDigest, patchedAt string) registry.Artifact {
	return registry.Artifact{
		Digest:       "sha256:entry-" + childDigest[7:19],
		ArtifactType: lineage.MediaTypeEntry,
		Annotations: map[string]string{
			lineage.AnnotationKind:        lineage.KindMetadata,
			lineage.AnnotationChildDigest: childDigest,
			lineage.AnnotationBaseDigest:  sourceBaseDigest,
			lineage.AnnotationBaseVersion: "1.0",
			lineage.AnnotationService:     "api",
			lineage.AnnotationPatchedAt:   patchedAt,
		},
	}
}

func deletedBaseRegistry(children ...registry.Artifact) *fakePatchRegistry {
	reg := &fakePatchRegistry{
		exists:    map[string]bool{},
		resolve:   map[string]string{},
		referrers: map[string][]registry.Artifact{},
	}
	// The base manifest is gone; only the tagged lineage index survives.
	reg.resolve[sourceRepo+":"+lineage.IndexTag] = sourceIndexDigest
	if len(children) > 0 {
		reg.exists[sourceRepo+":"+lineage.IndexTag] = true
		reg.referrers[sourceRepo+"@"+sourceIndexDigest] = children
	}
	for _, art := range children {
		child := art.Annotations[lineage.AnnotationChildDigest]
		reg.exists[sourceRepo+"@"+child] = true
	}
	return reg
}

func TestResolvePatchSourceUsesTheBaseWhenItStillExists(t *testing.T) {
	base := sourceRepo + "@" + sourceBaseDigest
	reg := &fakePatchRegistry{
		exists:    map[string]bool{base: true},
		resolve:   map[string]string{},
		referrers: map[string][]registry.Artifact{},
	}

	got, err := sourceApp().resolvePatchSource(context.Background(), reg, "api", base, "", catalog.New())
	if err != nil {
		t.Fatal(err)
	}
	if got.Ref != base {
		t.Errorf("ref = %q, want the base %q", got.Ref, base)
	}
	if got.BaseDigest != sourceBaseDigest {
		t.Errorf("base digest = %q, want %q", got.BaseDigest, sourceBaseDigest)
	}
	if reg.tagCalls != 0 {
		t.Errorf("Tags() called %d times; source resolution must never scan tags", reg.tagCalls)
	}
}

func TestResolvePatchSourceFallsBackToOldestChildWhenBaseDeleted(t *testing.T) {
	base := sourceRepo + "@" + sourceBaseDigest
	reg := deletedBaseRegistry(
		indexEntry(sourceNewChild, "2026-10-05T00:00:00Z"),
		indexEntry(sourceOldChild, "2026-10-01T00:00:00Z"),
	)

	got, err := sourceApp().resolvePatchSource(context.Background(), reg, "api", base, "", catalog.New())
	if err != nil {
		t.Fatal(err)
	}
	if want := sourceRepo + "@" + sourceOldChild; got.Ref != want {
		t.Errorf("ref = %q, want oldest child %q", got.Ref, want)
	}
	if got.BaseDigest != sourceBaseDigest {
		t.Errorf("base digest = %q, want %q", got.BaseDigest, sourceBaseDigest)
	}
	if !strings.Contains(got.Note, "oldest patched child") {
		t.Errorf("note = %q, want it to mention the oldest patched child", got.Note)
	}
	if reg.tagCalls != 0 {
		t.Errorf("Tags() called %d times; fallback must come from the lineage index, not tags", reg.tagCalls)
	}
}

func TestResolvePatchSourceSkipsChildrenDeletedFromTheRegistry(t *testing.T) {
	base := sourceRepo + "@" + sourceBaseDigest
	reg := deletedBaseRegistry(
		indexEntry(sourceOldChild, "2026-10-01T00:00:00Z"),
		indexEntry(sourceNewChild, "2026-10-05T00:00:00Z"),
	)
	// The oldest entry survived in the log but was pruned from the registry.
	delete(reg.exists, sourceRepo+"@"+sourceOldChild)

	got, err := sourceApp().resolvePatchSource(context.Background(), reg, "api", base, "", catalog.New())
	if err != nil {
		t.Fatal(err)
	}
	if want := sourceRepo + "@" + sourceNewChild; got.Ref != want {
		t.Errorf("ref = %q, want surviving child %q", got.Ref, want)
	}
}

func TestResolvePatchSourceFailsWhenNothingSurvives(t *testing.T) {
	base := sourceRepo + "@" + sourceBaseDigest
	reg := deletedBaseRegistry()

	_, err := sourceApp().resolvePatchSource(context.Background(), reg, "api", base, "", catalog.New())
	if err == nil {
		t.Fatal("expected an error when the base is gone and no child exists")
	}
	if !strings.Contains(err.Error(), "no patched child was found") {
		t.Errorf("error = %v, want it to say no patched child was found", err)
	}
	if reg.tagCalls != 0 {
		t.Errorf("Tags() called %d times; resolution must never scan tags", reg.tagCalls)
	}
}

// fakePatchRegistry is an in-memory registry that never consults tags unless a
// test explicitly asks for them.
type fakePatchRegistry struct {
	exists    map[string]bool
	resolve   map[string]string
	referrers map[string][]registry.Artifact
	tags      map[string][]string
	tagCalls  int
}

func (r *fakePatchRegistry) Resolve(_ context.Context, ref string) (string, error) {
	if digest, ok := r.resolve[ref]; ok {
		return digest, nil
	}
	return "", errors.New("no digest for " + ref)
}

func (r *fakePatchRegistry) Tags(_ context.Context, repository string) ([]string, error) {
	r.tagCalls++
	return r.tags[repository], nil
}

func (r *fakePatchRegistry) Exists(_ context.Context, ref string) (bool, error) {
	return r.exists[ref], nil
}

func (r *fakePatchRegistry) Catalog(context.Context) ([]string, error) { return nil, nil }

func (r *fakePatchRegistry) PushArtifact(context.Context, string, string, map[string]string) (string, error) {
	return "", errors.New("read-only fake")
}

func (r *fakePatchRegistry) EnsureArtifactTag(context.Context, string, string, string, map[string]string) (string, bool, error) {
	return "", false, errors.New("read-only fake")
}

func (r *fakePatchRegistry) Referrers(_ context.Context, subjectRef string) ([]registry.Artifact, error) {
	return r.referrers[subjectRef], nil
}

func (r *fakePatchRegistry) ReferrersOfType(_ context.Context, subjectRef, artifactType string) ([]registry.Artifact, error) {
	arts := r.referrers[subjectRef]
	if artifactType == "" {
		return arts, nil
	}
	out := make([]registry.Artifact, 0, len(arts))
	for _, art := range arts {
		if art.ArtifactType == artifactType {
			out = append(out, art)
		}
	}
	return out, nil
}

func (r *fakePatchRegistry) ArtifactAnnotations(context.Context, string) (map[string]string, error) {
	return nil, errors.New("annotations are inlined by the fake")
}
