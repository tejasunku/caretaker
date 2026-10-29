package cli

import (
	"testing"

	"github.com/tejasunku/caretaker/internal/lineage"
)

func entry(parent, child, base string) lineage.Entry {
	return lineage.Entry{
		ParentDigest: parent,
		ChildDigest:  child,
		BaseDigest:   base,
		SourceDigest: parent,
		PatchedAt:    "2026-10-29T00:00:00Z",
	}
}

func TestChildDepthsWalksGenerations(t *testing.T) {
	root, c1, c2 := "sha256:root", "sha256:c1", "sha256:c2"
	entries := []lineage.Entry{
		entry(root, c1, root),
		entry(c1, c2, root),
	}

	got := childDepths(entries, root)
	if got[c1] != 1 || got[c2] != 2 {
		t.Fatalf("depths from base = %v, want c1=1 c2=2", got)
	}

	got = childDepths(entries, c1)
	if len(got) != 1 || got[c2] != 1 {
		t.Fatalf("depths from mid-chain = %v, want only c2=1", got)
	}

	if got := childDepths(entries, c2); got != nil {
		t.Fatalf("leaf should have no children, got %v", got)
	}
}

func TestChildDepthsUsesBaseWhenParentLinkMissing(t *testing.T) {
	root, c1 := "sha256:root", "sha256:c1"
	entries := []lineage.Entry{{
		ChildDigest: c1,
		BaseDigest:  root,
		PatchedAt:   "2026-10-29T00:00:00Z",
	}}

	got := childDepths(entries, root)
	if got[c1] != 1 {
		t.Fatalf("depths = %v, want c1=1 via base fallback", got)
	}
}

func TestChildDepthsNoEntries(t *testing.T) {
	if got := childDepths(nil, "sha256:root"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
