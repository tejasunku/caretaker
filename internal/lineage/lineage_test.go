package lineage

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tejasunku/caretaker/internal/registry"
)

const (
	testRepo     = "localhost:5000/test/api"
	rootDigest   = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	child1Digest = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	child2Digest = "sha256:" + "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	rootRef      = testRepo + "@" + rootDigest
	child1Ref    = testRepo + "@" + child1Digest
	child2Ref    = testRepo + "@" + child2Digest
)

type memStore struct {
	refs map[string][]registry.Artifact
	anns map[string]map[string]string
	tags map[string]string
	n    int
}

func newMemStore() *memStore {
	return &memStore{
		refs: map[string][]registry.Artifact{},
		anns: map[string]map[string]string{},
		tags: map[string]string{},
	}
}

func (m *memStore) Exists(_ context.Context, ref string) (bool, error) {
	_, ok := m.tags[ref]
	return ok, nil
}

func (m *memStore) Resolve(_ context.Context, ref string) (string, error) {
	digest, ok := m.tags[ref]
	if !ok {
		return "", fmt.Errorf("resolve %s: not found", ref)
	}
	return digest, nil
}

func (m *memStore) PushArtifact(_ context.Context, subjectRef, artifactType string, annotations map[string]string) (string, error) {
	m.n++
	digest := fmt.Sprintf("sha256:%064x", m.n)
	m.refs[subjectRef] = append(m.refs[subjectRef], registry.Artifact{
		Digest:       digest,
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		ArtifactType: artifactType,
		Annotations:  annotations,
	})
	m.anns[digest] = annotations
	return digest, nil
}

func (m *memStore) EnsureArtifactTag(_ context.Context, repository, tag, artifactType string, annotations map[string]string) (string, bool, error) {
	ref := repository + ":" + tag
	if digest, ok := m.tags[ref]; ok {
		return digest, false, nil
	}
	digest, err := m.PushArtifact(context.Background(), repository+":"+tag, artifactType, annotations)
	if err != nil {
		return "", false, err
	}
	m.tags[ref] = digest
	return digest, true, nil
}

func (m *memStore) Referrers(_ context.Context, subjectRef string) ([]registry.Artifact, error) {
	return m.refs[subjectRef], nil
}

func (m *memStore) ReferrersOfType(_ context.Context, subjectRef, artifactType string) ([]registry.Artifact, error) {
	all := m.refs[subjectRef]
	if artifactType == "" {
		return all, nil
	}
	out := make([]registry.Artifact, 0, len(all))
	for _, art := range all {
		if art.ArtifactType == artifactType {
			out = append(out, art)
		}
	}
	return out, nil
}

func (m *memStore) ArtifactAnnotations(_ context.Context, ref string) (map[string]string, error) {
	_, digest, ok := strings.Cut(ref, "@")
	if !ok {
		return nil, fmt.Errorf("reference %q has no digest", ref)
	}
	ann, ok := m.anns[digest]
	if !ok {
		return nil, fmt.Errorf("artifact %s not found", digest)
	}
	return ann, nil
}

func TestRecordKeepsBaseAndParentSeparate(t *testing.T) {
	store := newMemStore()
	rec := &RecorderClient{Registry: store}
	ctx := context.Background()

	// root -> child1: base and parent are the same because the source is the
	// unpatched root image.
	if err := rec.Record(ctx, child1Ref, Metadata{
		Service:      "api",
		BaseDigest:   rootDigest,
		BaseVersion:  "1.0",
		SourceDigest: rootDigest,
		ParentDigest: rootDigest,
		PatchedAt:    "2026-10-20T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	// child1 -> child2: base stays at the root, parent moves to child1.
	if err := rec.Record(ctx, child2Ref, Metadata{
		Service:      "api",
		BaseDigest:   rootDigest,
		BaseVersion:  "1.0",
		SourceDigest: child1Digest,
		ParentDigest: child1Digest,
		PatchedAt:    "2026-10-21T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	meta, ok, err := rec.Metadata(ctx, child2Ref)
	if err != nil || !ok {
		t.Fatalf("metadata for child2 = %+v, ok=%v, err=%v", meta, ok, err)
	}
	if meta.BaseDigest != rootDigest {
		t.Errorf("child2 base = %q, want root %q", meta.BaseDigest, rootDigest)
	}
	if meta.ParentDigest != child1Digest || meta.SourceDigest != child1Digest {
		t.Errorf("child2 parent/source = %q/%q, want child1 %q", meta.ParentDigest, meta.SourceDigest, child1Digest)
	}

	base, version, ok, err := BaseFor(ctx, rec, child2Ref)
	if err != nil || !ok {
		t.Fatalf("BaseFor(child2) ok=%v err=%v", ok, err)
	}
	if base != rootDigest || version != "1.0" {
		t.Errorf("BaseFor(child2) = %q/%q, want %q/1.0", base, version, rootDigest)
	}

	rootChildren, err := rec.Children(ctx, rootRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootChildren) != 1 || rootChildren[0].Digest != child1Digest {
		t.Fatalf("root children = %+v, want child1 only", rootChildren)
	}
	if rootChildren[0].BaseDigest != rootDigest {
		t.Errorf("root child base = %q, want %q", rootChildren[0].BaseDigest, rootDigest)
	}

	child1Children, err := rec.Children(ctx, child1Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(child1Children) != 1 || child1Children[0].Digest != child2Digest {
		t.Fatalf("child1 children = %+v, want child2 only", child1Children)
	}
	if child1Children[0].BaseDigest != rootDigest {
		t.Errorf("child1 child base = %q, want %q", child1Children[0].BaseDigest, rootDigest)
	}

	if _, _, ok, err := BaseFor(ctx, rec, rootRef); err != nil || ok {
		t.Errorf("BaseFor(root) ok=%v err=%v, want no metadata", ok, err)
	}
}

func TestLogSurvivesParentDeletion(t *testing.T) {
	store := newMemStore()
	rec := &RecorderClient{Registry: store}
	ctx := context.Background()

	if err := rec.Record(ctx, child1Ref, Metadata{
		Service:      "api",
		BaseDigest:   rootDigest,
		BaseVersion:  "1.0",
		SourceDigest: rootDigest,
		ParentDigest: rootDigest,
		PatchedAt:    "2026-10-20T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := rec.Record(ctx, child2Ref, Metadata{
		Service:      "api",
		BaseDigest:   rootDigest,
		BaseVersion:  "1.0",
		SourceDigest: child1Digest,
		ParentDigest: child1Digest,
		PatchedAt:    "2026-10-21T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	// Deleting the root image takes its referrers list with it.
	delete(store.refs, rootRef)
	children, err := rec.Children(ctx, rootRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 0 {
		t.Fatalf("Children after base deletion = %+v, want none", children)
	}

	log, err := rec.Log(ctx, testRepo)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 {
		t.Fatalf("log = %+v, want 2 entries", log)
	}
	if log[0].ChildDigest != child1Digest || log[1].ChildDigest != child2Digest {
		t.Errorf("log order = %s, %s; want child1, child2", log[0].ChildDigest, log[1].ChildDigest)
	}
	if log[1].ParentDigest != child1Digest || log[1].BaseDigest != rootDigest {
		t.Errorf("log[1] parent/base = %s/%s", log[1].ParentDigest, log[1].BaseDigest)
	}

	byBase, err := rec.ChildrenForBase(ctx, testRepo, rootDigest)
	if err != nil {
		t.Fatal(err)
	}
	if len(byBase) != 2 {
		t.Fatalf("ChildrenForBase = %+v, want both children", byBase)
	}

	// A repository with no index yet has an empty log, not an error.
	empty, err := rec.Log(ctx, "localhost:5000/test/nope")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("log without index = %+v, want none", empty)
	}
	if _, err := rec.ChildrenForBase(ctx, "localhost:5000/test/nope", rootDigest); err != nil {
		t.Errorf("ChildrenForBase without index: %v", err)
	}
}
