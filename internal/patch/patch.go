// Package patch drives Copa (and optionally Trivy) to produce patched images.
// It never touches clusters.
package patch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/registry"
	"github.com/tejasunku/caretaker/internal/runner"
)

// Patcher invokes the external patch/scan tools.
type Patcher struct {
	Runner       runner.Runner
	Copa         string
	Trivy        string
	BuildkitAddr string
	Platform     string
	TagSuffix    string
	Report       string
	DryRun       bool
	Now          func() time.Time
	Lineage      lineage.Recorder
}

// Request describes one image to patch. Source is expected to be a digest
// reference; BaseDigest is the original parent when Source is a fallback child.
type Request struct {
	Service     string
	Source      string
	TargetTag   string
	BaseDigest  string
	BaseVersion string
	Force       bool
}

// Result describes one patched (or skipped) image.
type Result struct {
	Service      string
	Source       string
	Target       string
	TargetTag    string
	SourceDigest string
	TargetDigest string
	BaseDigest   string
	BaseVersion  string
	PatchedAt    string
	Skipped      bool
	Reason       string
	Err          error
}

// Patch patches sourceRef and tags the result as "<repo>:<targetTag>".
func (p *Patcher) Patch(ctx context.Context, sourceRef, targetTag string) error {
	args := []string{
		"patch",
		"-i", sourceRef,
		"-t", targetTag,
		"--addr", p.BuildkitAddr,
		"--platform", p.Platform,
		"--push",
		"--progress", "plain",
	}
	if p.Report != "" {
		args = append(args, "-r", p.Report)
	}
	return p.Runner.Run(ctx, p.Copa, args...)
}

// Scan writes a CycloneDX SBOM for ref using Trivy.
func (p *Patcher) Scan(ctx context.Context, ref, output string) error {
	return p.Runner.Run(ctx, p.Trivy,
		"image", "--quiet", "--format", "cyclonedx", "--output", output, ref)
}

// PatchRequest patches one digest-addressed source image and records lineage.
func (p *Patcher) PatchRequest(ctx context.Context, reg registry.Registry, req Request) (Result, error) {
	res := Result{
		Service:     req.Service,
		Source:      req.Source,
		BaseDigest:  req.BaseDigest,
		BaseVersion: req.BaseVersion,
	}

	parsed, err := image.Parse(req.Source)
	if err != nil {
		return res, err
	}
	if parsed.Digest == "" {
		return res, fmt.Errorf("%s: source %q has no digest; caretaker patches by digest", req.Service, req.Source)
	}

	res.SourceDigest, err = reg.Resolve(ctx, req.Source)
	if err != nil {
		return res, err
	}
	if res.BaseDigest == "" {
		res.BaseDigest = res.SourceDigest
	}

	targetTag := req.TargetTag
	if targetTag == "" {
		targetTag = p.pushTag(parsed, req.BaseVersion, res.SourceDigest)
	}
	res.TargetTag = targetTag
	res.Target = parsed.Repository + ":" + targetTag

	exists, err := reg.Exists(ctx, res.Target)
	if err != nil {
		return res, err
	}
	if exists && !req.Force {
		res.Skipped = true
		res.Reason = "patched tag already exists"
		res.TargetDigest, _ = reg.Resolve(ctx, res.Target)
		res.Target = parsed.Repository + "@" + res.TargetDigest
		return res, nil
	}

	if p.DryRun {
		res.Reason = "dry run"
		return res, nil
	}
	if err := p.Patch(ctx, req.Source, targetTag); err != nil {
		return res, err
	}
	res.TargetDigest, err = reg.Resolve(ctx, res.Target)
	if err != nil {
		return res, fmt.Errorf("%s: patched image not found after push: %w", req.Service, err)
	}
	res.Target = parsed.Repository + "@" + res.TargetDigest

	patchedAt := p.now().UTC().Format(time.RFC3339)
	res.PatchedAt = patchedAt
	if p.Lineage != nil {
		meta := lineage.Metadata{
			Service:      req.Service,
			BaseDigest:   res.BaseDigest,
			BaseVersion:  req.BaseVersion,
			SourceDigest: res.SourceDigest,
			ParentDigest: res.SourceDigest,
			PatchedAt:    patchedAt,
		}
		if err := p.Lineage.Record(ctx, res.Target, meta); err != nil {
			return res, fmt.Errorf("%s: image patched but lineage was not recorded: %w", req.Service, err)
		}
	}
	return res, nil
}

// PatchAll patches every request in order. Errors do not abort the batch; they
// are returned in the slice with the corresponding Result.
func (p *Patcher) PatchAll(ctx context.Context, reg registry.Registry, requests []Request) []Result {
	out := make([]Result, 0, len(requests))
	for _, req := range requests {
		res, err := p.PatchRequest(ctx, reg, req)
		if err != nil {
			res.Service = req.Service
			res.Err = err
			res.Reason = err.Error()
		}
		out = append(out, res)
	}
	return out
}

func (p *Patcher) pushTag(parsed image.Ref, baseVersion, sourceDigest string) string {
	base := strings.TrimSpace(baseVersion)
	if base == "" && parsed.Tag != "" {
		if v, ok := image.BaseVersion(parsed.Tag); ok {
			base = v
		} else {
			base = parsed.Tag
		}
	}
	if base == "" {
		base = "image"
	}
	suffix := p.TagSuffix
	if suffix == "" {
		suffix = "patched"
	}
	return fmt.Sprintf("%s-%s-%s-%s", base, suffix, p.now().UTC().Format("20060102-150405"), shortDigest(sourceDigest))
}

func (p *Patcher) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func shortDigest(digest string) string {
	d := strings.TrimPrefix(digest, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

// TargetRef is the digest-only reference stored in preferences.
func (r Result) TargetRef() string {
	if r.TargetDigest == "" {
		return r.Target
	}
	if parsed, err := image.Parse(r.Target); err == nil {
		return parsed.Repository + "@" + r.TargetDigest
	}
	return r.Target
}

// SourceRef is the digest-only base image reference stored in preferences.
func (r Result) SourceRef() string {
	if r.SourceDigest == "" {
		return r.Source
	}
	if parsed, err := image.Parse(r.Source); err == nil {
		return parsed.Repository + "@" + r.SourceDigest
	}
	return r.Source
}
