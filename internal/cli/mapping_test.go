package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/config"
)

const (
	mapTestBase   = "localhost:5000/test/api@sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mapTestOld    = "localhost:5000/test/api@sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mapTestNew    = "localhost:5000/test/api@sha256:" + "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	mapTestAlias  = "localhost:5000/test/api@sha256:" + "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	mapTestAlias2 = "localhost:5000/test/api@sha256:" + "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

func seedMappingFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preferences.yaml")
	seed := catalog.New()
	if err := seed.SetMapping("api", catalog.Mapping{
		Base: mapTestBase, Target: mapTestOld, BaseVersion: "1.0",
		PatchedAt: "2026-11-04T00:00:00Z", Note: "patched", Aliases: []string{mapTestAlias},
	}); err != nil {
		t.Fatal(err)
	}
	if err := seed.Save(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func runMappingSet(t *testing.T, path string, args ...string) {
	t.Helper()
	a := &App{cfg: config.Default(), Out: io.Discard, Err: io.Discard}
	cmd := a.mappingSetCmd()
	cmd.SetContext(context.Background())
	cmd.SetArgs(append([]string{"--file", path}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// mapping set used to replace the whole mapping, silently dropping baseVersion,
// aliases, patchedAt, and the note — which is how api lost its baseVersion.
func TestMappingSetPreservesUnspecifiedFields(t *testing.T) {
	path := seedMappingFile(t)

	runMappingSet(t, path, "--service", "api", "--base", mapTestBase, "--target", mapTestNew)

	got, err := catalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.MappingForBase("api", mapTestBase)
	if !ok {
		t.Fatal("mapping missing")
	}
	if m.Target != mapTestNew {
		t.Errorf("target = %q, want %q", m.Target, mapTestNew)
	}
	if m.BaseVersion != "1.0" {
		t.Errorf("baseVersion = %q, want preserved 1.0", m.BaseVersion)
	}
	if m.Note != "patched" {
		t.Errorf("note = %q, want preserved note", m.Note)
	}
	if len(m.Aliases) != 1 || m.Aliases[0] != mapTestAlias {
		t.Errorf("aliases = %v, want preserved alias", m.Aliases)
	}
	// The target changed by hand, so the old patchedAt no longer applies.
	if m.PatchedAt != "" {
		t.Errorf("patchedAt = %q, want cleared when the target changes", m.PatchedAt)
	}
}

func TestMappingSetAliasFlagReplacesList(t *testing.T) {
	path := seedMappingFile(t)

	runMappingSet(t, path, "--service", "api", "--base", mapTestBase, "--target", mapTestOld,
		"--alias", mapTestAlias2, "--note", "grouped")

	got, err := catalog.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.MappingForBase("api", mapTestBase)
	if !ok {
		t.Fatal("mapping missing")
	}
	if len(m.Aliases) != 1 || m.Aliases[0] != mapTestAlias2 {
		t.Errorf("aliases = %v, want [%s] (explicit --alias replaces the list)", m.Aliases, mapTestAlias2)
	}
	if m.Note != "grouped" {
		t.Errorf("note = %q, want grouped", m.Note)
	}
	if m.BaseVersion != "1.0" {
		t.Errorf("baseVersion = %q, want preserved 1.0", m.BaseVersion)
	}
	// Target unchanged: patchedAt survives.
	if m.PatchedAt != "2026-11-04T00:00:00Z" {
		t.Errorf("patchedAt = %q, want preserved when the target is unchanged", m.PatchedAt)
	}
}
