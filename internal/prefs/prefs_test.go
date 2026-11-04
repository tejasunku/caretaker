package prefs

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tejasunku/caretaker/internal/catalog"
)

const testRepo = "localhost:5000/test/caretaker-preferences"

type memArtifact struct {
	digest       string
	artifactType string
	payload      []byte
	annotations  map[string]string
	subject      string
}

type memStore struct {
	tags      map[string]string
	artifacts map[string]*memArtifact
	n         int
}

func newMemStore() *memStore {
	return &memStore{
		tags:      map[string]string{},
		artifacts: map[string]*memArtifact{},
	}
}

func (m *memStore) Exists(_ context.Context, ref string) (bool, error) {
	if _, ok := m.tags[ref]; ok {
		return true, nil
	}
	_, ok := m.artifacts[ref]
	return ok, nil
}

func (m *memStore) Resolve(_ context.Context, ref string) (string, error) {
	if d, ok := m.tags[ref]; ok {
		return d, nil
	}
	if _, ok := m.artifacts[ref]; ok {
		_, digest, _ := strings.Cut(ref, "@")
		return digest, nil
	}
	return "", fmt.Errorf("resolve %s: not found", ref)
}

func (m *memStore) EnsureArtifactTag(_ context.Context, repository, tag, artifactType string, annotations map[string]string) (string, bool, error) {
	ref := repository + ":" + tag
	if digest, ok := m.tags[ref]; ok {
		return digest, false, nil
	}
	m.n++
	digest := fmt.Sprintf("sha256:%064x", m.n)
	m.tags[ref] = digest
	m.artifacts[repository+"@"+digest] = &memArtifact{
		digest:       digest,
		artifactType: artifactType,
		annotations:  annotations,
	}
	return digest, true, nil
}

func (m *memStore) PushBlobArtifact(_ context.Context, subjectRef, artifactType string, payload []byte, annotations map[string]string) (string, error) {
	if ok, _ := m.Exists(context.Background(), subjectRef); !ok {
		return "", fmt.Errorf("subject %s not found", subjectRef)
	}
	m.n++
	digest := fmt.Sprintf("sha256:%064x", m.n)
	repository, _, _ := strings.Cut(subjectRef, "@")
	m.artifacts[repository+"@"+digest] = &memArtifact{
		digest:       digest,
		artifactType: artifactType,
		payload:      append([]byte(nil), payload...),
		annotations:  annotations,
		subject:      subjectRef,
	}
	return digest, nil
}

func (m *memStore) GetBlobArtifact(_ context.Context, ref string) ([]byte, error) {
	art, ok := m.artifacts[ref]
	if !ok {
		return nil, fmt.Errorf("artifact %s not found", ref)
	}
	if len(art.payload) == 0 {
		return nil, fmt.Errorf("artifact %s carries no payload", ref)
	}
	return append([]byte(nil), art.payload...), nil
}

func digestOf(fill string) string {
	return "localhost:5000/test/api@sha256:" + strings.Repeat(fill, 64)
}

func testDoc() *catalog.File {
	return &catalog.File{
		APIVersion: catalog.APIVersion,
		Kind:       catalog.Kind,
		Services: map[string]catalog.Service{
			"api": {
				Mappings: []catalog.Mapping{{
					Base:        digestOf("a"),
					Target:      digestOf("b"),
					BaseVersion: "1.0",
					Note:        "test",
				}},
			},
		},
	}
}

func newTestClient(store Store) *Client {
	return &Client{
		Registry:   store,
		Repository: testRepo,
		Writer:     "test-writer",
		Now: func() time.Time {
			return time.Date(2026, 11, 4, 12, 0, 0, 123456789, time.UTC)
		},
	}
}

func TestPushStoresVersionAndReturnsDigest(t *testing.T) {
	store := newMemStore()
	client := newTestClient(store)
	digest, err := client.Push(context.Background(), testDoc())
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest = %q, want sha256:...", digest)
	}

	indexDigest, ok := store.tags[testRepo+":"+IndexTag]
	if !ok || indexDigest == "" {
		t.Fatalf("index tag %s not created", IndexTag)
	}
	index := store.artifacts[testRepo+"@"+indexDigest]
	if index == nil || index.artifactType != MediaTypeIndex {
		t.Fatalf("index artifact = %+v, want artifactType %s", index, MediaTypeIndex)
	}

	version := store.artifacts[testRepo+"@"+digest]
	if version == nil {
		t.Fatalf("version artifact %s not stored", digest)
	}
	if version.artifactType != MediaTypePreferences {
		t.Errorf("artifactType = %q, want %q", version.artifactType, MediaTypePreferences)
	}
	if version.subject != testRepo+"@"+indexDigest {
		t.Errorf("subject = %q, want %q", version.subject, testRepo+"@"+indexDigest)
	}
	if got, want := version.annotations[AnnotationSetAt], "2026-11-04T12:00:00.123456789Z"; got != want {
		t.Errorf("set-at = %q, want %q", got, want)
	}
	if got := version.annotations[AnnotationWriter]; got != "test-writer" {
		t.Errorf("writer = %q, want test-writer", got)
	}
}

func TestPushRejectsInvalidPreferences(t *testing.T) {
	store := newMemStore()
	client := newTestClient(store)
	doc := testDoc()
	doc.Services["api"] = catalog.Service{Mappings: []catalog.Mapping{{Target: digestOf("b")}}}
	if _, err := client.Push(context.Background(), doc); err == nil {
		t.Fatal("push accepted a mapping without a base")
	}
	if len(store.artifacts) != 0 {
		t.Fatalf("pushed %d artifacts despite invalid document", len(store.artifacts))
	}
}

func TestPullRoundTripsVersion(t *testing.T) {
	store := newMemStore()
	client := newTestClient(store)
	doc := testDoc()
	digest, err := client.Push(context.Background(), doc)
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, input := range []string{digest, testRepo + "@" + digest} {
		got, ref, err := client.Pull(context.Background(), input)
		if err != nil {
			t.Fatalf("pull %q: %v", input, err)
		}
		if ref != testRepo+"@"+digest {
			t.Errorf("ref = %q, want %q", ref, testRepo+"@"+digest)
		}
		if !reflect.DeepEqual(got, doc) {
			t.Errorf("pull %q round-trip mismatch:\n got %#v\nwant %#v", input, got, doc)
		}
	}
}

func TestPullUnknownDigestFails(t *testing.T) {
	client := newTestClient(newMemStore())
	if _, _, err := client.Pull(context.Background(), "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("pull of an unknown digest succeeded")
	}
}

func TestPullRejectsTagReference(t *testing.T) {
	client := newTestClient(newMemStore())
	_, _, err := client.Pull(context.Background(), testRepo+":latest")
	if err == nil || !strings.Contains(err.Error(), "not a digest") {
		t.Fatalf("err = %v, want a digest-required error", err)
	}
}

func TestPushRequiresRepository(t *testing.T) {
	store := newMemStore()
	client := &Client{Registry: store}
	if _, err := client.Push(context.Background(), testDoc()); err == nil {
		t.Fatal("push without a repository succeeded")
	}
}
