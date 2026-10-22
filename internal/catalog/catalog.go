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

// Mapping points a source image at a preferred patched image.
type Mapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	Note   string `yaml:"note,omitempty"`
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

// Lookup resolves the preferred target for a running image.
func (f *File) Lookup(service, sourceImage string) (string, bool) {
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
		switch matchKind(m.Source, src) {
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

// Policy returns the service policy, defaulting to pinned.
func (f *File) Policy(service string) string {
	if p := f.Services[service].Policy; p != "" {
		return p
	}
	return PolicyPinned
}

// Set inserts or replaces the mapping for (service, source).
func (f *File) Set(service, source, target, note string) error {
	canonical, err := canonicalSource(source)
	if err != nil {
		return err
	}
	svc := f.Services[service]
	replaced := false
	for i := range svc.Mappings {
		if matchKind(svc.Mappings[i].Source, mustParse(canonical)) == exactMatch {
			svc.Mappings[i].Target = target
			svc.Mappings[i].Note = note
			replaced = true
			break
		}
	}
	if !replaced {
		svc.Mappings = append(svc.Mappings, Mapping{Source: canonical, Target: target, Note: note})
	}
	f.Services[service] = svc
	return nil
}

// Merge applies every mapping in updates on top of f. Later entries win.
func (f *File) Merge(updates *File) {
	for service, svc := range updates.Services {
		for _, m := range svc.Mappings {
			_ = f.Set(service, m.Source, m.Target, m.Note)
		}
		if svc.Policy != "" {
			current := f.Services[service]
			current.Policy = svc.Policy
			f.Services[service] = current
		}
	}
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

// matchKind prefers an exact digest/tag match, then a match on the numeric base
// version. The latter lets a preference for 1.0 apply to any 1.0-patched-* build.
func matchKind(raw string, src image.Ref) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
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

func canonicalSource(source string) (string, error) {
	if !strings.Contains(source, "/") {
		return strings.TrimSpace(source), nil
	}
	ref, err := image.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid source %q: %w", source, err)
	}
	return ref.String(), nil
}

func mustParse(s string) image.Ref {
	ref, err := image.Parse(s)
	if err != nil {
		return image.Ref{Tag: s}
	}
	return ref
}
