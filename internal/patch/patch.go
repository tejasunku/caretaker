// Package patch drives Copa (and optionally Trivy) to produce patched images.
// It never touches clusters.
package patch

import (
	"context"
	"fmt"
	"time"

	"github.com/tejasunku/caretaker/internal/image"
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
}

// Result describes one patched (or skipped) image.
type Result struct {
	Service      string
	Source       string
	Target       string
	SourceDigest string
	TargetDigest string
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

// PatchService patches the newest base tag of repository. When a patched tag
// for today already exists it is reused.
func (p *Patcher) PatchService(ctx context.Context, reg registry.Registry, service, repository string, force bool) (Result, error) {
	tags, err := reg.Tags(ctx, repository)
	if err != nil {
		return Result{Service: service}, err
	}
	baseTag, ok := image.NewestTag(tags, false)
	if !ok {
		return Result{Service: service}, fmt.Errorf("%s: no versioned tags in %s", service, repository)
	}

	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	targetTag := image.PatchedTag(baseTag, p.TagSuffix, now())
	return p.PatchImage(ctx, reg, service, repository+":"+baseTag, targetTag, force)
}

// PatchImage patches an explicit source image, tagging the result as targetTag
// in the source repository.
func (p *Patcher) PatchImage(ctx context.Context, reg registry.Registry, service, sourceRef, targetTag string, force bool) (Result, error) {
	res := Result{Service: service, Source: sourceRef}

	parsed, err := image.Parse(sourceRef)
	if err != nil {
		return res, err
	}
	res.Target = parsed.Repository + ":" + targetTag

	exists, err := reg.Exists(ctx, res.Target)
	if err != nil {
		return res, err
	}
	if exists && !force {
		res.Skipped = true
		res.Reason = "patched tag already exists"
		res.SourceDigest, _ = reg.Resolve(ctx, res.Source)
		res.TargetDigest, _ = reg.Resolve(ctx, res.Target)
		return res, nil
	}

	res.SourceDigest, err = reg.Resolve(ctx, res.Source)
	if err != nil {
		return res, err
	}
	if p.DryRun {
		res.Reason = "dry run"
		return res, nil
	}
	if err := p.Patch(ctx, res.Source, targetTag); err != nil {
		return res, err
	}
	res.TargetDigest, err = reg.Resolve(ctx, res.Target)
	if err != nil {
		return res, fmt.Errorf("%s: patched image not found after push: %w", service, err)
	}
	return res, nil
}

// PatchAll patches every service in order. Errors do not abort the batch; they
// are returned in the slice with the corresponding Result.
func (p *Patcher) PatchAll(ctx context.Context, reg registry.Registry, services []string, repository func(string) string, force bool) []Result {
	out := make([]Result, 0, len(services))
	for _, service := range services {
		res, err := p.PatchService(ctx, reg, service, repository(service), force)
		if err != nil {
			res.Service = service
			res.Err = err
			res.Reason = err.Error()
		}
		out = append(out, res)
	}
	return out
}

// TargetRef prefers tag+digest so the running deployment keeps its version
// readable while still being pinned to immutable bits.
func (r Result) TargetRef() string {
	if r.TargetDigest == "" {
		return r.Target
	}
	return r.Target + "@" + r.TargetDigest
}

// SourceRef returns the stable source key reflected in preferences. It is the
// versioned base image (repo:1.0), not a patched sibling, so any cluster on
// that version can resolve the same preference.
func (r Result) SourceRef() string { return r.Source }
