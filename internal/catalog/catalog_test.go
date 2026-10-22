package catalog

import "testing"

const (
	repo = "localhost:5000/test/api"
	src  = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dst  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestLookupByDigestAndTag(t *testing.T) {
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
	if _, ok := f.Lookup("api", repo+"@"+dst); ok {
		t.Fatal("unmapped digest should not resolve")
	}
	if _, ok := f.Lookup("web", repo+"@"+src); ok {
		t.Fatal("unknown service should not resolve")
	}
}

func TestSetReplacesExistingSource(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+":1.0", repo+":1.0-patched-20260101", ""); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("api", repo+":1.0", repo+":1.0-patched-20261016", "newer"); err != nil {
		t.Fatal(err)
	}
	if got := len(f.Services["api"].Mappings); got != 1 {
		t.Fatalf("expected 1 mapping, got %d", got)
	}
	if got, _ := f.Lookup("api", repo+":1.0"); got != repo+":1.0-patched-20261016" {
		t.Fatalf("got %q", got)
	}
}

func TestBareTagMapping(t *testing.T) {
	f := New()
	if err := f.Set("api", "1.0", repo+":1.0-patched-20261016", ""); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.Lookup("api", repo+":1.0@"+src); !ok || got != repo+":1.0-patched-20261016" {
		t.Fatalf("bare tag lookup = %q,%v", got, ok)
	}
}

func TestMergeLaterWins(t *testing.T) {
	base := New()
	_ = base.Set("api", repo+":1.0", repo+":1.0-patched-20260101", "")
	_ = base.Set("web", repo+":1.0", repo+":1.0-patched-20260101", "")

	updates := New()
	_ = updates.Set("api", repo+":1.0", repo+":1.0-patched-20261016", "")
	_ = updates.Set("worker", repo+":1.0", repo+":1.0-patched-20261016", "")

	base.Merge(updates)

	if got, _ := base.Lookup("api", repo+":1.0"); got != repo+":1.0-patched-20261016" {
		t.Errorf("api = %q", got)
	}
	if got, _ := base.Lookup("web", repo+":1.0"); got != repo+":1.0-patched-20260101" {
		t.Errorf("web = %q", got)
	}
	if got, _ := base.Lookup("worker", repo+":1.0"); got != repo+":1.0-patched-20261016" {
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

func TestBaseVersionMatch(t *testing.T) {
	f := New()
	_ = f.Set("api", repo+":1.0", repo+":1.0-patched-20261022", "")
	_ = f.Set("api", repo+":1.0-patched-20261001", repo+":1.0-patched-20261015", "")

	// A different 1.0 patched build should resolve to the base-version mapping.
	if got, ok := f.Lookup("api", repo+":1.0-patched-20261016@"+src); !ok || got != repo+":1.0-patched-20261022" {
		t.Fatalf("base-version lookup = %q,%v", got, ok)
	}
	// An exact tag mapping wins over the base-version mapping.
	if got, ok := f.Lookup("api", repo+":1.0-patched-20261001@"+src); !ok || got != repo+":1.0-patched-20261015" {
		t.Fatalf("exact lookup = %q,%v", got, ok)
	}
	// A different base version must not match.
	if _, ok := f.Lookup("api", repo+":2.0-patched-20261016@"+src); ok {
		t.Fatal("2.0 should not match a 1.0 preference")
	}
}
