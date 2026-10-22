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

func TestDigestOnlyMapping(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}
	m := f.Services["api"].Mappings[0]
	if m.Source != repo+"@"+src || m.Target != repo+"@"+dst {
		t.Fatalf("stored mapping = %+v", m)
	}
	if got, ok := f.Lookup("api", repo+":1.0@"+src); !ok || got != repo+"@"+dst {
		t.Fatalf("digest lookup = %q,%v", got, ok)
	}
	if _, ok := f.Lookup("api", repo+"@"+dst); ok {
		t.Fatal("unmapped digest should not resolve")
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
