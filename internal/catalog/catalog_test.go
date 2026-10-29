package catalog

import (
	"strings"
	"testing"
)

const (
	repo = "localhost:5000/test/api"
	src  = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dst  = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	dst2 = "sha256:" + "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestLookupByDigestOnly(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}

	if got, ok := f.Lookup("api", repo+"@"+src); !ok || got != repo+"@"+dst {
		t.Fatalf("digest lookup = %q,%v", got, ok)
	}
	// A running deployment normally carries repo:tag@digest; the digest should match.
	if got, ok := f.Lookup("api", repo+":1.0@"+src); !ok || got != repo+"@"+dst {
		t.Fatalf("tagged+digest lookup = %q,%v", got, ok)
	}
	if _, ok := f.Lookup("api", repo+":1.0"); ok {
		t.Fatal("a tag-only image must not resolve")
	}
	if _, ok := f.Lookup("api", repo+"@"+dst); ok {
		t.Fatal("unmapped digest should not resolve")
	}
	if _, ok := f.Lookup("web", repo+"@"+src); ok {
		t.Fatal("unknown service should not resolve")
	}
}

func TestSetReplacesExistingSource(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.SetMapping("api", Mapping{
		Source: repo + "@" + src, Target: repo + "@" + dst2, Note: "newer",
	}); err != nil {
		t.Fatal(err)
	}
	if got := len(f.Services["api"].Mappings); got != 1 {
		t.Fatalf("expected 1 mapping, got %d", got)
	}
	if got, _ := f.Lookup("api", repo+"@"+src); got != repo+"@"+dst2 {
		t.Fatalf("got %q", got)
	}
}

func TestSetRejectsTagOnlyRefs(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+":1.0", repo+"@"+dst, ""); err == nil {
		t.Fatal("expected tag source to be rejected")
	}
	if err := f.Set("api", repo+"@"+src, repo+":1.0", ""); err == nil {
		t.Fatal("expected tag target to be rejected")
	}
	if err := f.SetMapping("api", Mapping{
		Source: repo + "@" + src, Target: repo + "@" + dst, BaseDigest: repo + ":1.0",
	}); err == nil {
		t.Fatal("expected tag base digest to be rejected")
	}
}

func TestDigestOnlyMapping(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}
	m := f.Services["api"].Mappings[0]
	if m.Source != repo+"@"+src || m.Target != repo+"@"+dst {
		t.Fatalf("stored mapping = %+v", m)
	}
}

func TestMergeLaterWins(t *testing.T) {
	base := New()
	if err := base.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}
	if err := base.Set("web", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}

	updates := New()
	if err := updates.Set("api", repo+"@"+src, repo+"@"+dst2, ""); err != nil {
		t.Fatal(err)
	}
	if err := updates.Set("worker", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}

	if err := base.Merge(updates); err != nil {
		t.Fatal(err)
	}

	if got, _ := base.Lookup("api", repo+"@"+src); got != repo+"@"+dst2 {
		t.Errorf("api = %q", got)
	}
	if got, _ := base.Lookup("web", repo+"@"+src); got != repo+"@"+dst {
		t.Errorf("web = %q", got)
	}
	if got, _ := base.Lookup("worker", repo+"@"+src); got != repo+"@"+dst {
		t.Errorf("worker = %q", got)
	}
}

func TestPolicyDefault(t *testing.T) {
	f := New()
	if got := f.Policy("api"); got != PolicyPinned {
		t.Fatalf("default policy = %q", got)
	}
	svc := f.Services["api"]
	svc.Policy = PolicyNewest
	f.Services["api"] = svc
	if got := f.Policy("api"); got != PolicyNewest {
		t.Fatalf("policy = %q", got)
	}
}

func TestDigestMatchIgnoresTag(t *testing.T) {
	f := New()
	_ = f.Set("api", repo+"@"+src, repo+"@"+dst, "")

	// Any tag carrying the mapped digest should resolve.
	if got, ok := f.Lookup("api", repo+":1.0-patched-20261016@"+src); !ok || got != repo+"@"+dst {
		t.Fatalf("lookup = %q,%v", got, ok)
	}
	// A different digest with the same tag must not.
	if _, ok := f.Lookup("api", repo+":1.0-patched-20261016@"+dst); ok {
		t.Fatal("a different digest should not match")
	}
}

func TestLookupTagsIsOptIn(t *testing.T) {
	f := New()
	// Hand-build a legacy tag mapping; Set no longer accepts tags.
	f.Services["api"] = Service{Mappings: []Mapping{{
		Source: repo + ":1.0", Target: repo + "@" + dst,
	}}}

	if _, ok := f.Lookup("api", repo+":1.0"); ok {
		t.Fatal("digest-only Lookup must ignore tag mappings")
	}
	if got, ok := f.LookupTags("api", repo+":1.0"); !ok || got != repo+"@"+dst {
		t.Fatalf("LookupTags = %q,%v", got, ok)
	}
}

func TestLatestSourceAndSourceForTarget(t *testing.T) {
	f := New()
	if err := f.SetMapping("api", Mapping{
		Source: repo + "@" + src, Target: repo + "@" + dst,
		BaseDigest: repo + "@" + dst2, BaseVersion: "1.0", PatchedAt: "2026-10-20T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	if got, ok := f.LatestSource("api"); !ok || got != repo+"@"+dst2 {
		t.Fatalf("LatestSource = %q,%v", got, ok)
	}
	if got, ok := f.LatestTarget("api"); !ok || got != repo+"@"+dst {
		t.Fatalf("LatestTarget = %q,%v", got, ok)
	}
	if got, ok := f.SourceForTarget("api", repo+"@"+dst); !ok || got != repo+"@"+dst2 {
		t.Fatalf("SourceForTarget = %q,%v", got, ok)
	}
	if _, ok := f.SourceForTarget("api", repo+"@"+src); ok {
		t.Fatal("unknown target should not resolve")
	}
}

func TestMappingForSource(t *testing.T) {
	f := New()
	if err := f.SetMapping("api", Mapping{
		Source: repo + "@" + src, Target: repo + "@" + dst, BaseVersion: "1.0",
	}); err != nil {
		t.Fatal(err)
	}
	m, ok := f.MappingForSource("api", repo+":1.0@"+src)
	if !ok || !strings.HasPrefix(m.Target, repo+"@") {
		t.Fatalf("MappingForSource = %+v,%v", m, ok)
	}
}
