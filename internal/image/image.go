// Package image parses OCI image references and understands caretaker's
// "<version>-patched-<date>" tag convention.
package image

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
)

// Ref is a parsed image reference. Exactly one of Tag or Digest is normally
// set; a reference authored as repo:tag@digest keeps both.
type Ref struct {
	Repository string
	Tag        string
	Digest     string
}

// Parse accepts repo, repo:tag, repo@sha256:..., or repo:tag@sha256:...
func Parse(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Ref{}, fmt.Errorf("empty image reference")
	}

	base, digest, hasDigest := strings.Cut(s, "@")
	ref, err := name.ParseReference(base, name.WithDefaultTag("latest"))
	if err != nil {
		return Ref{}, fmt.Errorf("parse image %q: %w", s, err)
	}

	out := Ref{Repository: ref.Context().Name()}
	if hasExplicitTag(base) {
		out.Tag = ref.Identifier()
	} else if !hasDigest {
		out.Tag = "latest"
	}
	if hasDigest {
		dref, err := name.NewDigest(out.Repository + "@" + digest)
		if err != nil {
			return Ref{}, fmt.Errorf("parse digest in %q: %w", s, err)
		}
		out.Digest = dref.DigestStr()
	}
	return out, nil
}

func hasExplicitTag(s string) bool {
	return strings.LastIndex(s, ":") > strings.LastIndex(s, "/")
}

// RepositoryOnly parses a repository (no tag or digest).
func RepositoryOnly(s string) (string, error) {
	repo, err := name.NewRepository(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("parse repository %q: %w", s, err)
	}
	return repo.Name(), nil
}

// String renders the canonical form: digest wins when present.
func (r Ref) String() string {
	if r.Digest != "" {
		return r.Repository + "@" + r.Digest
	}
	return r.Repository + ":" + r.Tag
}

// Tagged renders repo:tag, even when a digest is present.
func (r Ref) Tagged() string {
	return r.Repository + ":" + r.Tag
}

// Pinned renders the most descriptive immutable form: repo:tag@digest when a
// tag is known, otherwise repo@digest.
func (r Ref) Pinned() string {
	if r.Digest == "" {
		return r.String()
	}
	if r.Tag != "" {
		return r.Repository + ":" + r.Tag + "@" + r.Digest
	}
	return r.Repository + "@" + r.Digest
}

// SameImage reports whether two refs identify the same bits. Digests are the
// identity: a tag is never used to equate a digest-bearing ref with a ref that
// has no digest.
func (r Ref) SameImage(other Ref) bool {
	if r.Repository != other.Repository {
		return false
	}
	if r.Digest != "" || other.Digest != "" {
		return r.Digest != "" && r.Digest == other.Digest
	}
	return r.Tag != "" && r.Tag == other.Tag
}

var versionRE = regexp.MustCompile(`^(\d+(?:\.\d+)*)(-patched-\d+)?$`)

// BaseVersion returns the numeric version of a tag, ignoring a -patched-<date>
// suffix. It reports false for tags that do not look versioned.
func BaseVersion(tag string) (string, bool) {
	m := versionRE.FindStringSubmatch(tag)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// IsPatched reports whether a tag carries the -patched- suffix.
func IsPatched(tag string) bool {
	return strings.Contains(tag, "-patched-")
}

// PatchedTag builds "<base>-patched-<YYYYMMDD>" in UTC.
func PatchedTag(base string, suffix string, at time.Time) string {
	if suffix == "" {
		suffix = "patched"
	}
	return base + "-" + suffix + "-" + at.UTC().Format("20060102")
}

// CompareVersions compares dotted numeric versions. It returns -1, 0, or 1.
// Non-numeric segments compare as zero.
func CompareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		av, bv := 0, 0
		if i < len(as) {
			av, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bv, _ = strconv.Atoi(bs[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// NewestTag returns the highest versioned tag in tags. Patched tags are
// consider ed only when includePatched is true.
func NewestTag(tags []string, includePatched bool) (string, bool) {
	best := ""
	bestVersion := ""
	for _, tag := range tags {
		if !includePatched && IsPatched(tag) {
			continue
		}
		v, ok := BaseVersion(tag)
		if !ok {
			continue
		}
		if best == "" || CompareVersions(v, bestVersion) > 0 {
			best, bestVersion = tag, v
		}
	}
	return best, best != ""
}

// LatestPatched returns the lexicographically last patched tag for base, which
// is ordered by date because of the YYYYMMDD suffix.
func LatestPatched(tags []string, base string) (string, bool) {
	prefix := base + "-patched-"
	best := ""
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) && tag > best {
			best = tag
		}
	}
	return best, best != ""
}
