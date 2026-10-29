// Package catalog stores the universal preferred-patch map: which patched
// image a service should move to for a given source image. It is deliberately
// file-backed for now; the lookup/merge API is what a CRD or control cluster
// would later implement.
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
	"strings"

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

// Mapping points a source image at a preferred patched image. Source is the
// image that was actually patched; BaseDigest is the original parent when the
// source was itself a patched child (fallback lineage).
type Mapping struct {
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	BaseDigest  string `yaml:"baseDigest,omitempty"`
	BaseVersion string `yaml:"baseVersion,omitempty"`
	PatchedAt   string `yaml:"patchedAt,omitempty"`
	Note        string `yaml:"note,omitempty"`
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

// Lookup resolves the preferred target for a running image by digest only.
// A tag never matches unless LookupTags is used.
func (f *File) Lookup(service, sourceImage string) (string, bool) {
	return f.lookup(service, sourceImage, false)
}

// LookupTags is the opt-in compatibility path for tag-based preferences.
func (f *File) LookupTags(service, sourceImage string) (string, bool) {
	return f.lookup(service, sourceImage, true)
}

func (f *File) lookup(service, sourceImage string, allowTags bool) (string, bool) {
	svc, ok := f.Services[service]
	if !ok {
		return "", false
	}
	src, err := image.Parse(sourceImage)
	if err != nil {
		return "", false
	}
	var baseTarget string
	baseFound := false
	for _, m := range svc.Mappings {
		switch matchKind(m.Source, src, allowTags) {
		case exactMatch:
			return m.Target, true
		case baseMatch:
			if !baseFound {
				baseTarget, baseFound = m.Target, true
			}
		}
	}
	return baseTarget, baseFound
}

// LatestSource returns the source of the most recently added mapping. It is the
// digest-first replacement for scanning the registry for the newest tag.
func (f *File) LatestSource(service string) (string, bool) {
	svc, ok := f.Services[service]
	if !ok || len(svc.Mappings) == 0 {
		return "", false
	}
	m := svc.Mappings[len(svc.Mappings)-1]
	if m.BaseDigest != "" {
		return m.BaseDigest, true
	}
	return m.Source, true
}

// LatestTarget returns the target of the most recently added mapping.
func (f *File) LatestTarget(service string) (string, bool) {
	svc, ok := f.Services[service]
	if !ok || len(svc.Mappings) == 0 {
		return "", false
	}
	return svc.Mappings[len(svc.Mappings)-1].Target, true
}

// SourceForTarget returns the source (or original base) that produced target.
func (f *File) SourceForTarget(service, target string) (string, bool) {
	svc, ok := f.Services[service]
	if !ok {
		return "", false
	}
	want, err := image.Parse(target)
	if err != nil {
		return "", false
	}
	for _, m := range svc.Mappings {
		got, err := image.Parse(m.Target)
		if err != nil || got.Digest == "" || want.Digest == "" || got.Digest != want.Digest {
			continue
		}
		if m.BaseDigest != "" {
			return m.BaseDigest, true
		}
		return m.Source, true
	}
	return "", false
}

// MappingForSource returns the mapping whose source matches sourceImage.
func (f *File) MappingForSource(service, sourceImage string) (Mapping, bool) {
	want, err := image.Parse(sourceImage)
	if err != nil {
		return Mapping{}, false
	}
	for _, m := range f.Services[service].Mappings {
		got, err := image.Parse(m.Source)
		if err == nil && got.SameImage(want) {
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

// Set inserts or replaces the mapping for (service, source).
func (f *File) Set(service, source, target, note string) error {
	return f.SetMapping(service, Mapping{Source: source, Target: target, Note: note})
}

// SetMapping inserts or replaces a full mapping, keyed by source digest.
func (f *File) SetMapping(service string, m Mapping) error {
	source, err := canonicalDigestRef(m.Source)
	if err != nil {
		return err
	}
	target, err := canonicalDigestRef(m.Target)
	if err != nil {
		return err
	}
	base := ""
	if m.BaseDigest != "" {
		base, err = canonicalDigestRef(m.BaseDigest)
		if err != nil {
			return err
		}
	}
	m.Source, m.Target, m.BaseDigest = source, target, base

	svc := f.Services[service]
	for i := range svc.Mappings {
		if sameDigest(svc.Mappings[i].Source, m.Source) {
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

const (
	noMatch = iota
	baseMatch
	exactMatch
)

// matchKind prefers an exact digest match. Tag/base-version matching is only
// available when allowTags is set for backwards compatibility.
func matchKind(raw string, src image.Ref, allowTags bool) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return noMatch
	}
	if !allowTags {
		if !strings.Contains(raw, "/") {
			return noMatch
		}
		mapped, err := image.Parse(raw)
		if err != nil || mapped.Repository != src.Repository {
			return noMatch
		}
		if mapped.Digest != "" && src.Digest != "" && mapped.Digest == src.Digest {
			return exactMatch
		}
		return noMatch
	}
	// A bare tag (no slash) matches on tag alone.
	if !strings.Contains(raw, "/") {
		return tagMatch(raw, src.Tag)
	}
	mapped, err := image.Parse(raw)
	if err != nil || mapped.Repository != src.Repository {
		return noMatch
	}
	if mapped.Digest != "" && src.Digest != "" {
		if mapped.Digest == src.Digest {
			return exactMatch
		}
		return noMatch
	}
	return tagMatch(mapped.Tag, src.Tag)
}

func tagMatch(mapped, running string) int {
	if mapped == "" || running == "" {
		return noMatch
	}
	if mapped == running {
		return exactMatch
	}
	mb, mok := image.BaseVersion(mapped)
	rb, rok := image.BaseVersion(running)
	if mok && rok && mb == rb {
		return baseMatch
	}
	return noMatch
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
