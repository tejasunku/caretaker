package patch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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

func TestPatchRequestRecordsDescendantLineage(t *testing.T) {
	rootDigest := "sha256:" + strings.Repeat("e", 64)
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
		Service: "api", Source: source, TargetTag: targetTag,
		BaseDigest: rootDigest, BaseVersion: "1.0",
	}); err != nil {
		t.Fatal(err)
	}
	if rec.meta.BaseDigest != rootDigest {
		t.Errorf("base digest = %q, want root %q", rec.meta.BaseDigest, rootDigest)
	}
	if rec.meta.ParentDigest != oldDigest || rec.meta.SourceDigest != oldDigest {
		t.Errorf("parent/source = %q/%q, want immediate source %q", rec.meta.ParentDigest, rec.meta.SourceDigest, oldDigest)
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

func TestPatchRequestSkipIfClean(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{source: oldDigest, targetRef: newDigest}}
	run := &scanRunner{report: `{"Results":[{"Target":"x"}]}`}
	p := &Patcher{
		Runner: run, Copa: "copa", Trivy: "trivy", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", SkipIfClean: true,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped || res.Reason != "no vulnerabilities found" {
		t.Fatalf("result = %+v", res)
	}
	for _, call := range run.snapshot() {
		if call[0] == "copa" {
			t.Fatalf("copa should not run for a clean image: %v", call)
		}
	}
	if got := strings.Join(run.snapshot()[0], " "); !strings.Contains(got, "trivy image") || !strings.Contains(got, source) {
		t.Fatalf("scan invocation = %q", got)
	}
}

func TestPatchRequestSkipIfCleanPatchesWhenVulnerable(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{source: oldDigest, targetRef: newDigest}}
	run := &scanRunner{report: `{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-2026-0001"}]}]}`}
	p := &Patcher{
		Runner: run, Copa: "copa", Trivy: "trivy", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", SkipIfClean: true,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped || res.TargetDigest != newDigest {
		t.Fatalf("result = %+v", res)
	}
	sawCopa := false
	for _, call := range run.snapshot() {
		if call[0] == "copa" {
			sawCopa = true
		}
	}
	if !sawCopa {
		t.Fatalf("copa should run when vulnerabilities exist: %v", run.snapshot())
	}
}

func TestPatchRequestSkipIfCleanFailsOpen(t *testing.T) {
	reg := &fakeRegistry{digests: map[string]string{source: oldDigest, targetRef: newDigest}}
	run := &scanRunner{scanErr: errors.New("trivy unavailable")}
	p := &Patcher{
		Runner: run, Copa: "copa", Trivy: "trivy", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", SkipIfClean: true,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchRequest(context.Background(), reg, Request{
		Service: "api", Source: source, TargetTag: targetTag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped || !strings.Contains(res.Reason, "scan failed") {
		t.Fatalf("result = %+v", res)
	}
	sawCopa := false
	for _, call := range run.snapshot() {
		if call[0] == "copa" {
			sawCopa = true
		}
	}
	if !sawCopa {
		t.Fatalf("copa should run when the scan fails: %v", run.snapshot())
	}
}

func TestPatchAllRunsConcurrentlyAndPreservesOrder(t *testing.T) {
	digests := map[string]string{}
	requests := make([]Request, 0, 5)
	for i, letter := range []string{"a", "b", "c", "d", "e"} {
		src := "sha256:" + strings.Repeat(letter, 64)
		tag := fmt.Sprintf("target-%d", i)
		digests[repo+"@"+src] = src
		digests[repo+":"+tag] = src
		requests = append(requests, Request{Service: fmt.Sprintf("svc-%d", i), Source: repo + "@" + src, TargetTag: tag})
	}
	run := &concurrencyRunner{delay: 10 * time.Millisecond}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", Concurrency: 3,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	results := p.PatchAll(context.Background(), &fakeRegistry{digests: digests}, requests)
	if len(results) != len(requests) {
		t.Fatalf("results = %+v", results)
	}
	for i, res := range results {
		if res.Service != requests[i].Service || res.Err != nil {
			t.Errorf("result[%d] = %+v", i, res)
		}
	}
	callCount, max := run.stats()
	if callCount != len(requests) {
		t.Errorf("copa calls = %d, want %d", callCount, len(requests))
	}
	if max > 3 {
		t.Errorf("max in-flight = %d, want <= 3", max)
	}
	if max < 2 {
		t.Errorf("expected concurrent patching, max in-flight = %d", max)
	}
}

type scanRunner struct {
	mu      sync.Mutex
	calls   [][]string
	report  string
	scanErr error
}

func (r *scanRunner) Run(_ context.Context, name string, args ...string) error {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string{name}, args...))
	r.mu.Unlock()
	if name == "trivy" {
		if r.scanErr != nil {
			return r.scanErr
		}
		for i, arg := range args {
			if arg == "--output" && i+1 < len(args) {
				return os.WriteFile(args[i+1], []byte(r.report), 0o600)
			}
		}
	}
	return nil
}

func (r *scanRunner) snapshot() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]string, len(r.calls))
	copy(out, r.calls)
	return out
}

type concurrencyRunner struct {
	mu       sync.Mutex
	inFlight int
	max      int
	calls    int
	delay    time.Duration
}

func (r *concurrencyRunner) Run(context.Context, string, ...string) error {
	r.mu.Lock()
	r.inFlight++
	r.calls++
	if r.inFlight > r.max {
		r.max = r.inFlight
	}
	r.mu.Unlock()
	time.Sleep(r.delay)
	r.mu.Lock()
	r.inFlight--
	r.mu.Unlock()
	return nil
}

func (r *concurrencyRunner) stats() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.max
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
