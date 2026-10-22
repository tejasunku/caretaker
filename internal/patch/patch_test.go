package patch

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tejasunku/caretaker/internal/runner"
)

const (
	repo       = "localhost:5000/test/api"
	oldDigest  = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newDigest  = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	targetTag  = "1.0-patched-20261020"
	targetRepo = repo + ":" + targetTag
)

func TestPatchServiceBuildsCopaInvocation(t *testing.T) {
	reg := &fakeRegistry{
		tags:    []string{"1.0", "2.0-patched-20260101"},
		digests: map[string]string{repo + ":1.0": oldDigest, targetRepo: newDigest},
	}
	run := &runner.Print{}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", Report: "trivy.json",
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchService(context.Background(), reg, "api", repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != targetRepo || res.TargetDigest != newDigest {
		t.Fatalf("result = %+v", res)
	}
	if len(run.Calls) != 1 {
		t.Fatalf("calls = %v", run.Calls)
	}
	got := strings.Join(run.Calls[0], " ")
	for _, want := range []string{
		"copa patch", "-i " + repo + ":1.0", "-t " + targetTag,
		"--push", "-r trivy.json",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("invocation %q missing %q", got, want)
		}
	}
}

func TestPatchServiceSkipsExistingTarget(t *testing.T) {
	reg := &fakeRegistry{
		tags:     []string{"1.0"},
		digests:  map[string]string{repo + ":1.0": oldDigest, targetRepo: newDigest},
		existing: map[string]bool{targetRepo: true},
	}
	run := &runner.Print{}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched",
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchService(context.Background(), reg, "api", repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Fatalf("expected skip, got %+v", res)
	}
	if len(run.Calls) != 0 {
		t.Errorf("copa should not run, calls = %v", run.Calls)
	}
}

func TestPatchServiceDryRunDoesNotInvokeCopa(t *testing.T) {
	reg := &fakeRegistry{
		tags:    []string{"1.0"},
		digests: map[string]string{repo + ":1.0": oldDigest},
	}
	run := &runner.Print{}
	p := &Patcher{
		Runner: run, Copa: "copa", BuildkitAddr: "tcp://127.0.0.1:12345",
		Platform: "linux/amd64", TagSuffix: "patched", DryRun: true,
		Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
	}

	res, err := p.PatchService(context.Background(), reg, "api", repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != "dry run" || len(run.Calls) != 0 {
		t.Fatalf("result = %+v calls = %v", res, run.Calls)
	}
}

type fakeRegistry struct {
	tags     []string
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

func (r *fakeRegistry) Tags(context.Context, string) ([]string, error) { return r.tags, nil }

func (r *fakeRegistry) Exists(_ context.Context, ref string) (bool, error) {
	return r.existing[ref], nil
}

func (r *fakeRegistry) Catalog(context.Context) ([]string, error) { return nil, nil }
