package catalog

import (
	"os"
	"path/filepath"
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
		t.Fatal("an unmapped digest should not resolve")
	}
	if _, ok := f.Lookup("web", repo+"@"+src); ok {
		t.Fatal("unknown service should not resolve")
	}
}

func TestSetReplacesExistingBase(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.SetMapping("api", Mapping{
		Base: repo + "@" + src, Target: repo + "@" + dst2, Note: "newer",
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
		t.Fatal("expected tag base to be rejected")
	}
	if err := f.Set("api", repo+"@"+src, repo+":1.0", ""); err == nil {
		t.Fatal("expected tag target to be rejected")
	}
}

func TestDigestOnlyMapping(t *testing.T) {
	f := New()
	if err := f.Set("api", repo+"@"+src, repo+"@"+dst, ""); err != nil {
		t.Fatal(err)
	}
	m := f.Services["api"].Mappings[0]
	if m.Base != repo+"@"+src || m.Target != repo+"@"+dst {
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

func TestLatestBaseAndBaseForTarget(t *testing.T) {
	f := New()
	if err := f.SetMapping("api", Mapping{
		Base: repo + "@" + src, Target: repo + "@" + dst,
		BaseVersion: "1.0", PatchedAt: "2026-10-20T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	if got, ok := f.LatestBase("api"); !ok || got != repo+"@"+src {
		t.Fatalf("LatestBase = %q,%v", got, ok)
	}
	if got, ok := f.LatestTarget("api"); !ok || got != repo+"@"+dst {
		t.Fatalf("LatestTarget = %q,%v", got, ok)
	}
	if got, ok := f.BaseForTarget("api", repo+"@"+dst); !ok || got != repo+"@"+src {
		t.Fatalf("BaseForTarget = %q,%v", got, ok)
	}
	if _, ok := f.BaseForTarget("api", repo+"@"+src); ok {
		t.Fatal("unknown target should not resolve")
	}
}

func TestMappingForBase(t *testing.T) {
	f := New()
	if err := f.SetMapping("api", Mapping{
		Base: repo + "@" + src, Target: repo + "@" + dst, BaseVersion: "1.0",
	}); err != nil {
		t.Fatal(err)
	}
	m, ok := f.MappingForBase("api", repo+":1.0@"+src)
	if !ok || !strings.HasPrefix(m.Target, repo+"@") || m.BaseVersion != "1.0" {
		t.Fatalf("MappingForBase = %+v,%v", m, ok)
	}
}

func TestLookupMatchesAliases(t *testing.T) {
	f := New()
	alias := repo + "@" + dst2
	if err := f.SetMapping("api", Mapping{
		Base:        repo + "@" + src,
		Aliases:     []string{alias},
		Target:      repo + "@" + dst,
		BaseVersion: "1.0",
	}); err != nil {
		t.Fatal(err)
	}

	// The feature build still resolves.
	if got, ok := f.Lookup("api", repo+"@"+src); !ok || got != repo+"@"+dst {
		t.Fatalf("base lookup = %q,%v", got, ok)
	}
	// An alias (older patched version) resolves to the same target.
	if got, ok := f.Lookup("api", alias); !ok || got != repo+"@"+dst {
		t.Fatalf("alias lookup = %q,%v", got, ok)
	}
	// Cross-service recognition also sees the alias.
	if svc, got, ok := f.LookupAnyService(alias); !ok || svc != "api" || got != repo+"@"+dst {
		t.Fatalf("LookupAnyService(alias) = %q,%q,%v", svc, got, ok)
	}
	// MappingForBase matches the alias and keeps the canonical Base.
	if m, ok := f.MappingForBase("api", alias); !ok || m.Base != repo+"@"+src {
		t.Fatalf("MappingForBase(alias) = %+v,%v", m, ok)
	}
	// A tag-only alias is rejected.
	err := f.SetMapping("api", Mapping{Base: repo + "@" + src, Aliases: []string{repo + ":1.0"}, Target: repo + "@" + dst})
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tag alias error = %v", err)
	}
}

func writePrefs(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preferences.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAcceptsCurrentFormat(t *testing.T) {
	path := writePrefs(t, `apiVersion: caretaker/v1alpha2
kind: PatchPreferences
services:
  api:
    mappings:
      - base: `+repo+`@`+src+`
        target: `+repo+`@`+dst+`
        baseVersion: "1.0"
`)
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.APIVersion != APIVersion {
		t.Errorf("apiVersion = %q, want %q", f.APIVersion, APIVersion)
	}
	if got, ok := f.Lookup("api", repo+"@"+src); !ok || got != repo+"@"+dst {
		t.Errorf("lookup = %q,%v", got, ok)
	}
}

// A v1alpha1 source-keyed file must fail loudly: it used to load with an empty
// base on every mapping, so every lookup failed with no error at all.
func TestLoadRejectsLegacySourceKeyedFile(t *testing.T) {
	path := writePrefs(t, `apiVersion: caretaker/v1alpha1
kind: PatchPreferences
services:
  api:
    mappings:
      - source: `+repo+`@`+src+`
        target: `+repo+`@`+dst+`
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "apiVersion") {
		t.Fatalf("error = %v, want unsupported apiVersion", err)
	}
}

func TestLoadRejectsUnknownAPIVersion(t *testing.T) {
	path := writePrefs(t, `apiVersion: caretaker/v2
kind: PatchPreferences
services: {}
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "caretaker/v2") {
		t.Fatalf("error = %v, want unsupported apiVersion", err)
	}
}

func TestLoadRejectsMappingWithoutBase(t *testing.T) {
	path := writePrefs(t, `apiVersion: caretaker/v1alpha2
kind: PatchPreferences
services:
  api:
    mappings:
      - target: `+repo+`@`+dst+`
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "base and target are required") {
		t.Fatalf("error = %v, want missing base", err)
	}
}

func TestLoadEmptyFileYieldsEmptyCatalog(t *testing.T) {
	path := writePrefs(t, "")
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Services) != 0 {
		t.Errorf("services = %v, want empty", f.Services)
	}
}

func TestSetMappingPreservesUnspecifiedFields(t *testing.T) {
	alias := repo + "@" + "sha256:" + strings.Repeat("e", 64)
	f := New()
	if err := f.SetMapping("api", Mapping{
		Base: repo + "@" + src, Target: repo + "@" + dst,
		BaseVersion: "1.0", PatchedAt: "2026-11-04T00:00:00Z",
		Note: "patched", Aliases: []string{alias},
	}); err != nil {
		t.Fatal(err)
	}

	// A partial update (base + target only) must not drop the other fields.
	if err := f.SetMapping("api", Mapping{Base: repo + "@" + src, Target: repo + "@" + dst2}); err != nil {
		t.Fatal(err)
	}
	m, ok := f.MappingForBase("api", repo+"@"+src)
	if !ok {
		t.Fatal("mapping missing after partial update")
	}
	if m.Target != repo+"@"+dst2 {
		t.Errorf("target = %q, want the new target", m.Target)
	}
	if m.BaseVersion != "1.0" {
		t.Errorf("baseVersion = %q, want preserved 1.0", m.BaseVersion)
	}
	if m.PatchedAt != "2026-11-04T00:00:00Z" {
		t.Errorf("patchedAt = %q, want preserved timestamp", m.PatchedAt)
	}
	if m.Note != "patched" {
		t.Errorf("note = %q, want preserved note", m.Note)
	}
	if len(m.Aliases) != 1 || m.Aliases[0] != alias {
		t.Errorf("aliases = %v, want preserved [%s]", m.Aliases, alias)
	}

	// An explicit empty (non-nil) alias list clears the aliases.
	if err := f.SetMapping("api", Mapping{Base: repo + "@" + src, Target: repo + "@" + dst2, Aliases: []string{}}); err != nil {
		t.Fatal(err)
	}
	m, _ = f.MappingForBase("api", repo+"@"+src)
	if len(m.Aliases) != 0 {
		t.Errorf("aliases = %v, want cleared", m.Aliases)
	}

	// An explicit baseVersion replaces the preserved one.
	if err := f.SetMapping("api", Mapping{Base: repo + "@" + src, Target: repo + "@" + dst2, BaseVersion: "2.0"}); err != nil {
		t.Fatal(err)
	}
	m, _ = f.MappingForBase("api", repo+"@"+src)
	if m.BaseVersion != "2.0" {
		t.Errorf("baseVersion = %q, want 2.0", m.BaseVersion)
	}
}
