package image

import (
	"strings"
	"testing"
	"time"
)

const digestA = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const digestB = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestParse(t *testing.T) {
	tests := []struct {
		in     string
		repo   string
		tag    string
		digest string
	}{
		{"localhost:5000/test/api:1.0", "localhost:5000/test/api", "1.0", ""},
		{"localhost:5000/test/api", "localhost:5000/test/api", "latest", ""},
		{"localhost:5000/test/api@" + digestA, "localhost:5000/test/api", "", digestA},
		{"localhost:5000/test/api:1.0@" + digestA, "localhost:5000/test/api", "1.0", digestA},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got.Repository != tt.repo || got.Tag != tt.tag || got.Digest != tt.digest {
			t.Errorf("Parse(%q) = %+v, want repo=%q tag=%q digest=%q", tt.in, got, tt.repo, tt.tag, tt.digest)
		}
	}
}

func TestRefSameImage(t *testing.T) {
	tagged := Ref{Repository: "r", Tag: "1.0", Digest: digestA}
	pinned := Ref{Repository: "r", Tag: "1.0", Digest: digestA}
	other := Ref{Repository: "r", Tag: "1.0-patched-20260101", Digest: digestB}

	if !tagged.SameImage(pinned) {
		t.Error("same digest should match")
	}
	if tagged.SameImage(other) {
		t.Error("different digests should not match")
	}
	if !(Ref{Repository: "r", Tag: "1.0"}).SameImage(Ref{Repository: "r", Tag: "1.0"}) {
		t.Error("same tag with no digests should match")
	}
}

func TestBaseVersion(t *testing.T) {
	tests := map[string]string{
		"1.0":                    "1.0",
		"1.0-patched-20261016":   "1.0",
		"2.3.4-patched-20260101": "2.3.4",
		"latest":                 "",
		"v1.0":                   "",
	}
	for tag, want := range tests {
		got, ok := BaseVersion(tag)
		if want == "" {
			if ok {
				t.Errorf("BaseVersion(%q) = %q, want not ok", tag, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("BaseVersion(%q) = %q,%v, want %q", tag, got, ok, want)
		}
	}
}

func TestPatchedTag(t *testing.T) {
	got := PatchedTag("1.0", "patched", time.Date(2026, 10, 16, 23, 0, 0, 0, time.UTC))
	if got != "1.0-patched-20261016" {
		t.Fatalf("got %q", got)
	}
}

func TestNewestTag(t *testing.T) {
	tags := []string{"1.0", "1.10", "1.2", "2.0-patched-20260101", "latest"}
	if got, ok := NewestTag(tags, false); !ok || got != "1.10" {
		t.Errorf("NewestTag = %q,%v, want 1.10", got, ok)
	}
	if got, ok := NewestTag(tags, true); !ok || got != "2.0-patched-20260101" {
		t.Errorf("NewestTag(includePatched) = %q,%v", got, ok)
	}
}

func TestLatestPatched(t *testing.T) {
	tags := []string{"1.0-patched-20260101", "1.0-patched-20261016", "1.0", "1.1-patched-20260101"}
	if got, ok := LatestPatched(tags, "1.0"); !ok || got != "1.0-patched-20261016" {
		t.Errorf("LatestPatched = %q,%v", got, ok)
	}
	if _, ok := LatestPatched(tags, "9.9"); ok {
		t.Error("expected no match")
	}
}

func TestCompareVersions(t *testing.T) {
	if CompareVersions("1.10", "1.2") != 1 {
		t.Error("1.10 should be greater than 1.2")
	}
	if CompareVersions("2.0", "2.0.0") != 0 {
		t.Error("2.0 and 2.0.0 should compare equal")
	}
	if CompareVersions("1.0", "1.0.1") != -1 {
		t.Error("1.0 should be less than 1.0.1")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse("   "); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-reference error, got %v", err)
	}
}
