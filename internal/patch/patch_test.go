package patch

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/runner"
)

const (
	repo          = "localhost:5000/test/api"
	oldDigest     = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newDigest     = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	source        = repo + "@" + oldDigest
	targetTag     = "1.0-patched-20261020"
	targetRef     = repo + ":" + targetTag
	generatedTag  = "1.0-patched-20261020-000000-aaaaaaaaaaaa"
	generatedRef  = repo + ":" + generatedTag
	patchedAtTime = "2026-10-20T00:00:00Z"
)

func TestPatchRequestBuildsDigestFirstCopaInvocation(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{
		source:       oldDigest,
		generatedRef: newDigest,
	}}
	run := &runner.Print{}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", Report: "trivy.json",
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, BaseVersion: "1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != repo+"@"+newDigest || res.TargetDigest != newDigest {
		t.Fatalf("result = %+v", res)
	}
	if res.TargetTag != generatedTag {
		t.Errorf("target tag = %q, want %q", res.TargetTag, generatedTag)
	}
	if res.SourceDigest != oldDigest || res.BaseDigest != oldDigest {
		t.Errorf("source/base digest = %q/%q", res.SourceDigest, res.BaseDigest)
	}
	if got := res.SourceRef(); got != source {
		t.Errorf("SourceRef = %q, want %q", got, source)
	}
	if got := res.TargetRef(); got != repo+"@"+newDigest {
		t.Errorf("TargetRef = %q, want %q", got, repo+"@"+newDigest)
	}
	if len(run.Calls) != 1 {
		t.Fatalf("calls = %v", run.Calls)
	}
	got := strings.Join(run.Calls[0], " ")
	for _, want := range []string{
		"copa patch", "-i " + source, "-t " + generatedTag,
		"--push", "-r trivy.json",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("invocation %q missing %q", got, want)
		}
	}
}

func TestPatchRequestSkipsExistingTarget(t *testing.T) {
	reg := &fakeRegistry{
		digests:  map[string]string{source: oldDigest, targetRef: newDigest},
		existing: map[string]bool{targetRef: true},
	}
	run := &runner.Print{}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched",
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Fatalf("expected skip, got %+v", res)
	}
	if res.Target != repo+"@"+newDigest || res.TargetDigest != newDigest {
		t.Fatalf("result = %+v", res)
	}
	if len(run.Calls) != 0 {
		t.Errorf("copa should not run, calls = %v", run.Calls)
	}
}

func TestPatchRequestDryRunDoesNotInvokeCopa(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{source: oldDigest}}
	run := &runner.Print{}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", DryRun: true,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "dry run" || len(run.Calls) != 0 {
		t.Fatalf("result = %+v calls = %v", res, run.Calls)
	}
}

func TestPatchRequestRejectsTagSource(t *testing.T) {
	reg := &fakeRegistry{}
	p := &Patcher{Runner: &runner.Print{}, Copa: "copa"}
	_, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: repo + ":1.0",
	})
	if err == nil || !strings.Contains(err.Error(), "no digest") {
		t.Fatalf("expected digest requirement, got %v", err)
	}
}

func TestPatchRequestRecordsLineage(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{
		source: oldDigest, targetRef: newDigest,
	}}
	rec := &fakeLineage{}
	p := &Patcher{
		Runner: &runner.Print{}, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", Lineage: rec,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	if _, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag, BaseVersion: "1.0",
	}); err != nil {
		t.Fatal(err)
	}
	if rec.child != repo+"@"+newDigest {
		t.Errorf("lineage child = %q", rec.child)
	}
	if rec.meta.Service != "api" || rec.meta.BaseDigest != oldDigest ||
		rec.meta.SourceDigest != oldDigest || rec.meta.ParentDigest != oldDigest ||
		rec.meta.BaseVersion != "1.0" || rec.meta.PatchedAt != patchedAtTime {
		t.Errorf("lineage meta = %+v", rec.meta)
	}
}

func TestPatchRequestLineageFailureReturnsError(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{
		source: oldDigest, targetRef: newDigest,
	}}
	rec := &fakeLineage{err: fmt.Errorf("registry unavailable")}
	p := &Patcher{
		Runner: &runner.Print{}, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", Lineage: rec,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag,
	})
	if err == nil || !strings.Contains(err.Error(), "lineage") {
		t.Fatalf("expected lineage error, got %v", err)
	}
	if res.TargetDigest != newDigest {
		t.Fatalf("result = %+v", res)
	}
}

func TestPatchAllContinuesAfterFailure(t *testing.T) {
	missing := repo + "@sha256:" + strings.Repeat("d", 64)
	reg := &fakeRegistry{digests: map[string]string{source: oldDigest}}
	p := &Patcher{
		Runner: &runner.Print{}, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", DryRun: true,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	results := p.PatchAll(context.Background(), reg, []Request{
		{Service: "api", Source: source, BaseVersion: "1.0"},
		{Service: "web", Source: missing},
	})
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Err != nil {
		t.Errorf("first result error = %v", results[0].Err)
	}
	if results[1].Err == nil || results[1].Service != "web" {
		t.Errorf("second result = %+v", results[1])
	}
}

type fakeRegistry struct {
	digests  map[string]string
	existing map[string]bool
}

func (r *fakeRegistry) Resolve(_ context.Context, ref string) (string, error) {
	d, ok := r.digests[ref]
	if !ok {
		return "", fmt.Errorf("no digest for %s", ref)
	}
	return d, nil
}

func (r *fakeRegistry) Tags(context.Context, string) ([]string, error) { return nil, nil }

func (r *fakeRegistry) Exists(_ context.Context, ref string) (bool, error) {
	return r.existing[ref], nil
}

func (r *fakeRegistry) Catalog(context.Context) ([]string, error) { return nil, nil }

type fakeLineage struct {
	child string
	meta  lineage.Metadata
	err   error
}

func (f *fakeLineage) Record(_ context.Context, childRef string, meta lineage.Metadata) error {
	f.child = childRef
	f.meta = meta
	return f.err
}
