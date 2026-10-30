// Package catalog stores the universal preferred-patch map: which patched
// image a service should move to for a given feature build (base) image. It is
// deliberately file-backed for now; the lookup/merge API is what a CRD or
// control cluster would later implement.
package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/tejasunku/caretaker/internal/image"
)

const (
	APIVersion = "caretaker/v1alpha1"
	Kind       = "PatchPreferences"

	PolicyPinned = "pinned"
	PolicyNewest = "newest"
)

// File is the on-disk preferences document.
type File struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Services   map[string]Service `yaml:"services"`
}

// Service holds mappings for one service.
type Service struct {
	Policy   string    `yaml:"policy,omitempty"`
	Mappings []Mapping `yaml:"mappings,omitempty"`
}

// Mapping states the preferred patched image for one feature build. Base is
// the feature-build (base) digest and is the mapping key; Target is the
// preferred patched digest. Aliases optionally lists other digests (typically
// older patched versions of the same feature set) that should also resolve to
// Target, so a rollback to an older patched image is still recognized and
// bumped forward. BaseVersion/PatchedAt are human metadata.
type Mapping struct {
	Base        string   `yaml:"base"`
	Aliases     []string `yaml:"aliases,omitempty"`
	Target      string   `yaml:"target"`
	BaseVersion string   `yaml:"baseVersion,omitempty"`
	PatchedAt   string   `yaml:"patchedAt,omitempty"`
	Note        string   `yaml:"note,omitempty"`
}

// matchesBase reports whether image is this mapping's feature build or one of
// its aliases.
func (m Mapping) matchesBase(image string) bool {
	if sameDigest(m.Base, image) {
		return true
	}
	for _, a := range m.Aliases {
		if sameDigest(a, image) {
			return true
		}
	}
	return false
}

// New returns an empty catalog.
func New() *File {
	return &File{APIVersion: APIVersion, Kind: Kind, Services: map[string]Service{}}
}

// Load reads a preferences file. A missing file yields an empty catalog.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read preferences %s: %w", path, err)
	}
	out := New()
	if err := yaml.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("parse preferences %s: %w", path, err)
	}
	if out.Services == nil {
		out.Services = map[string]Service{}
	}
	if out.APIVersion == "" {
		out.APIVersion = APIVersion
	}
	if out.Kind == "" {
		out.Kind = Kind
	}
	return out, nil
}

// Save writes the catalog, creating parent directories.
func (f *File) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create preferences directory: %w", err)
	}
	var buf bytes.Buffer
	if err := f.Encode(&buf); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write preferences %s: %w", path, err)
	}
	return nil
}

// Encode writes the catalog as YAML with two-space indentation.
func (f *File) Encode(w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()
	if err := enc.Encode(f); err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
	return nil
}

// Lookup resolves the preferred target for a feature-build image by digest
// only. The key of every mapping is the feature build's digest; any listed
// alias resolves to the same target.
func (f *File) Lookup(service, baseImage string) (string, bool) {
	for _, m := range f.Services[service].Mappings {
		if m.matchesBase(baseImage) {
			return m.Target, true
		}
	}
	return "", false
}

// LatestBase returns the feature build of the most recently added mapping. It
// is the digest-first replacement for scanning the registry for the newest tag.
func (f *File) LatestBase(service string) (string, bool) {
	svc, ok := f.Services[service]
	if !ok || len(svc.Mappings) == 0 {
		return "", false
	}
	return svc.Mappings[len(svc.Mappings)-1].Base, true
}

// LatestTarget returns the target of the most recently added mapping.
func (f *File) LatestTarget(service string) (string, bool) {
	svc, ok := f.Services[service]
	if !ok || len(svc.Mappings) == 0 {
		return "", false
	}
	return svc.Mappings[len(svc.Mappings)-1].Target, true
}

// LookupAnyService finds the preferred target for a feature-build image
// across every service, keyed only by digest. It lets the admission webhook
// recognize an image by what it is rather than by the container or Deployment
// that happens to carry it. If two services share a base digest the first in
// sorted order wins.
func (f *File) LookupAnyService(baseImage string) (service, target string, ok bool) {
	for _, name := range f.ServiceNames() {
		if t, found := f.Lookup(name, baseImage); found {
			return name, t, true
		}
	}
	return "", "", false
}

// BaseForTargetAnyService finds the feature build whose preferred target is
// targetImage, across every service.
func (f *File) BaseForTargetAnyService(targetImage string) (service, base string, ok bool) {
	for _, name := range f.ServiceNames() {
		if b, found := f.BaseForTarget(name, targetImage); found {
			return name, b, true
		}
	}
	return "", "", false
}

// MappingAnyService finds the mapping whose feature build (or alias) matches
// image, across every service. It returns the owning service and the full
// mapping so callers can read the canonical base.
func (f *File) MappingAnyService(image string) (service string, m Mapping, ok bool) {
	for _, name := range f.ServiceNames() {
		for _, mm := range f.Services[name].Mappings {
			if mm.matchesBase(image) {
				return name, mm, true
			}
		}
	}
	return "", Mapping{}, false
}

// BaseForTarget returns the feature build whose preferred target is target.
// It is how a running patched image finds its base without lineage metadata.
func (f *File) BaseForTarget(service, target string) (string, bool) {
	want, err := image.Parse(target)
	if err != nil || want.Digest == "" {
		return "", false
	}
	for _, m := range f.Services[service].Mappings {
		got, err := image.Parse(m.Target)
		if err == nil && got.Digest != "" && got.Digest == want.Digest {
			return m.Base, true
		}
	}
	return "", false
}

// MappingForBase returns the mapping whose feature build (or alias) matches
// baseImage.
func (f *File) MappingForBase(service, baseImage string) (Mapping, bool) {
	if _, err := image.Parse(baseImage); err != nil {
		return Mapping{}, false
	}
	for _, m := range f.Services[service].Mappings {
		if m.matchesBase(baseImage) {
			return m, true
		}
	}
	return Mapping{}, false
}

// Policy returns the service policy, defaulting to pinned.
func (f *File) Policy(service string) string {
	if p := f.Services[service].Policy; p != "" {
		return p
	}
	return PolicyPinned
}

// Set inserts or replaces the mapping for (service, base).
func (f *File) Set(service, base, target, note string) error {
	return f.SetMapping(service, Mapping{Base: base, Target: target, Note: note})
}

// SetMapping inserts or replaces a mapping, keyed by the feature-build digest.
func (f *File) SetMapping(service string, m Mapping) error {
	base, err := canonicalDigestRef(m.Base)
	if err != nil {
		return err
	}
	target, err := canonicalDigestRef(m.Target)
	if err != nil {
		return err
	}
	m.Base, m.Target = base, target
	if len(m.Aliases) > 0 {
		aliases := make([]string, 0, len(m.Aliases))
		for _, a := range m.Aliases {
			c, err := canonicalDigestRef(a)
			if err != nil {
				return err
			}
			aliases = append(aliases, c)
		}
		m.Aliases = aliases
	}

	svc := f.Services[service]
	for i := range svc.Mappings {
		if sameDigest(svc.Mappings[i].Base, m.Base) {
			svc.Mappings[i] = m
			f.Services[service] = svc
			return nil
		}
	}
	svc.Mappings = append(svc.Mappings, m)
	f.Services[service] = svc
	return nil
}

// Merge applies every mapping in updates on top of f. Later entries win.
func (f *File) Merge(updates *File) error {
	for service, svc := range updates.Services {
		for _, m := range svc.Mappings {
			if err := f.SetMapping(service, m); err != nil {
				return err
			}
		}
		if svc.Policy != "" {
			current := f.Services[service]
			current.Policy = svc.Policy
			f.Services[service] = current
		}
	}
	return nil
}

// ServiceNames returns the service names in sorted order.
func (f *File) ServiceNames() []string {
	out := make([]string, 0, len(f.Services))
	for name := range f.Services {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func canonicalDigestRef(raw string) (string, error) {
	ref, err := image.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid reference %q: %w", raw, err)
	}
	if ref.Digest == "" {
		return "", fmt.Errorf("reference %q has no digest; caretaker is digest-first (use --allow-tags for tag input)", raw)
	}
	return ref.Repository + "@" + ref.Digest, nil
}

func sameDigest(a, b string) bool {
	ar, err := image.Parse(a)
	if err != nil {
		return false
	}
	br, err := image.Parse(b)
	if err != nil {
		return false
	}
	return ar.SameImage(br)
}
