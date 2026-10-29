package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/patch"
	"github.com/tejasunku/caretaker/internal/registry"
)

func (a *App) patchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patch",
		Short: "Patch images with Copa",
	}
	cmd.AddCommand(a.patchImageCmd(), a.patchAllCmd())
	return cmd
}

func (a *App) patchImageCmd() *cobra.Command {
	var service, source, tag, baseDigest, baseVersion, report, preferences string
	var writeMapping, force, dryRun, cveOnly, skipIfClean bool
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Patch one service image",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireService(service); err != nil {
				return err
			}
			if cveOnly && report == "" && a.cfg.Patch.Report == "" {
				return errString("--cve-only requires --report")
			}
			if !cmd.Flags().Changed("skip-if-clean") {
				skipIfClean = a.cfg.Patch.SkipIfClean
			}
			ctx := cmd.Context()
			reg := a.registryClient()
			prefs, _, err := a.loadPreferences(preferences)
			if err != nil {
				return err
			}

			resolved, err := a.resolvePatchSource(ctx, reg, service, source, baseVersion, prefs)
			if err != nil {
				return err
			}
			if baseDigest != "" {
				parsed, err := image.Parse(baseDigest)
				if err != nil || parsed.Digest == "" {
					return fmt.Errorf("--base-digest must be repo@sha256:...: %w", err)
				}
				resolved.BaseDigest = parsed.Digest
			}

			req := patch.Request{
				Service:     service,
				Source:      resolved.Ref,
				TargetTag:   tag,
				BaseDigest:  resolved.BaseDigest,
				BaseVersion: resolved.BaseVersion,
				Force:       force,
			}
			res, err := a.patcher(patchOptions{
				Report:      report,
				DryRun:      dryRun,
				SkipIfClean: skipIfClean,
			}).PatchRequest(ctx, reg, req)
			if err != nil {
				return err
			}
			if resolved.Note != "" {
				res.Reason = strings.TrimSpace(res.Reason + " " + resolved.Note)
			}
			a.printPatchResults([]patch.Result{res})
			if writeMapping {
				return a.writePatchMapping(preferences, []patch.Result{res})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&service, "service", "", "service name (required)")
	cmd.Flags().StringVar(&source, "source", "", "source image digest (default: patch.sources or the last preferred source)")
	cmd.Flags().StringVar(&tag, "tag", "", "push tag for the patched image (default: generated)")
	cmd.Flags().StringVar(&baseDigest, "base-digest", "", "original base digest (default: the source digest)")
	cmd.Flags().StringVar(&baseVersion, "base-version", "", "base version label recorded in lineage")
	cmd.Flags().StringVar(&report, "report", "", "vulnerability report file or directory for Copa (CVE-only patching)")
	cmd.Flags().BoolVar(&cveOnly, "cve-only", false, "patch only reported CVEs (requires --report)")
	cmd.Flags().BoolVar(&skipIfClean, "skip-if-clean", false, "scan with Trivy and skip when no vulnerabilities are found (default: patch.skipIfClean)")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file to update")
	cmd.Flags().BoolVar(&writeMapping, "write-mapping", false, "record the result in the preferences file")
	cmd.Flags().BoolVar(&force, "force", false, "rebuild even if the generated push tag exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func (a *App) patchAllCmd() *cobra.Command {
	var servicesSel, source, baseVersion, report, preferences string
	var writeMapping, force, dryRun, cveOnly, skipIfClean bool
	var concurrency int
	cmd := &cobra.Command{
		Use:   "all",
		Short: "Patch every selected service image",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cveOnly && report == "" && a.cfg.Patch.Report == "" {
				return errString("--cve-only requires --report")
			}
			ctx := cmd.Context()
			services, err := a.cfg.ResolveServices(servicesSel)
			if err != nil {
				return err
			}
			if source != "" && len(services) != 1 {
				return errString("--source can only be used when patching one service")
			}
			if !cmd.Flags().Changed("skip-if-clean") {
				skipIfClean = a.cfg.Patch.SkipIfClean
			}
			if !cmd.Flags().Changed("concurrency") {
				concurrency = a.cfg.Patch.Concurrency
			}
			if concurrency < 1 {
				return errString("--concurrency must be at least 1")
			}
			reg := a.registryClient()
			prefs, _, err := a.loadPreferences(preferences)
			if err != nil {
				return err
			}

			requests := make([]patch.Request, 0, len(services))
			pre := make([]patch.Result, 0)
			for _, svc := range services {
				resolved, err := a.resolvePatchSource(ctx, reg, svc, source, baseVersion, prefs)
				if err != nil {
					pre = append(pre, patch.Result{Service: svc, Err: err, Reason: err.Error()})
					continue
				}
				requests = append(requests, patch.Request{
					Service:     svc,
					Source:      resolved.Ref,
					BaseDigest:  resolved.BaseDigest,
					BaseVersion: resolved.BaseVersion,
					Force:       force,
				})
			}

			p := a.patcher(patchOptions{
				Report:      report,
				DryRun:      dryRun,
				SkipIfClean: skipIfClean,
				Concurrency: concurrency,
			})
			results := append(pre, p.PatchAll(ctx, reg, requests)...)
			a.printPatchResults(results)
			if writeMapping {
				if err := a.writePatchMapping(preferences, results); err != nil {
					return err
				}
			}
			return patchResultsError(results)
		},
	}
	cmd.Flags().StringVar(&servicesSel, "services", "all", "comma-separated services, or all")
	cmd.Flags().StringVar(&source, "source", "", "source image digest (only with one service)")
	cmd.Flags().StringVar(&baseVersion, "base-version", "", "base version label recorded in lineage")
	cmd.Flags().StringVar(&report, "report", "", "vulnerability report file or directory for Copa (CVE-only patching)")
	cmd.Flags().BoolVar(&cveOnly, "cve-only", false, "patch only reported CVEs (requires --report)")
	cmd.Flags().BoolVar(&skipIfClean, "skip-if-clean", false, "scan with Trivy and skip when no vulnerabilities are found (default: patch.skipIfClean)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 0, "number of images to patch at once (default: patch.concurrency or 1)")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file to update")
	cmd.Flags().BoolVar(&writeMapping, "write-mapping", false, "record results in the preferences file")
	cmd.Flags().BoolVar(&force, "force", false, "rebuild even if a generated push tag exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

type resolvedSource struct {
	Ref         string
	BaseDigest  string
	BaseVersion string
	Note        string
}

// patchRegistry is the registry surface patch source resolution needs: the
// read-only registry API plus the lineage store behind the index log.
type patchRegistry interface {
	registry.Registry
	lineage.Store
}

// resolvePatchSource picks the image Copa will patch. The base (feature build)
// is used while it still exists in the registry; once it has been deleted we
// fall back to the oldest patched child recorded for it in the lineage index.
// There is deliberately no policy flag for this: one behavior, oldest child.
func (a *App) resolvePatchSource(ctx context.Context, reg patchRegistry, service, explicit, baseVersion string, prefs *catalog.File) (resolvedSource, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		raw = strings.TrimSpace(a.cfg.Patch.Sources[service])
	}
	if raw == "" {
		if src, ok := prefs.LatestSource(service); ok {
			raw = src
		}
	}
	if raw == "" {
		if !a.allowTags() {
			return resolvedSource{}, fmt.Errorf("%s: no source digest configured; pass --source %s@sha256:... or set patch.sources.%s", service, a.cfg.Repository(service), service)
		}
		tags, err := reg.Tags(ctx, a.cfg.Repository(service))
		if err != nil {
			return resolvedSource{}, err
		}
		base, ok := image.NewestTag(tags, false)
		if !ok {
			return resolvedSource{}, fmt.Errorf("%s: no versioned tags in %s", service, a.cfg.Repository(service))
		}
		raw = a.cfg.Repository(service) + ":" + base
	}

	parsed, err := image.Parse(raw)
	if err != nil {
		return resolvedSource{}, fmt.Errorf("%s: %w", service, err)
	}
	version := strings.TrimSpace(baseVersion)
	if version == "" && parsed.Tag != "" {
		if v, ok := image.BaseVersion(parsed.Tag); ok {
			version = v
		}
	}
	if parsed.Digest == "" {
		if !a.allowTags() {
			return resolvedSource{}, fmt.Errorf("%s: source %q is a tag; caretaker is digest-first (use --allow-tags to opt in)", service, raw)
		}
		digest, err := reg.Resolve(ctx, raw)
		if err != nil {
			return resolvedSource{}, fmt.Errorf("%s: %w", service, err)
		}
		parsed.Digest = digest
	}
	ref := parsed.Repository + "@" + parsed.Digest
	mapping, hasMapping := prefs.MappingForSource(service, ref)

	// The base is the original image at the root of the patch chain. The
	// parent is the immediate image we are about to patch. They are the same
	// only when the source is unpatched.
	baseDigest, baseVersion, note := "", "", ""
	if base, v, ok, err := lineage.BaseFor(ctx, &lineage.RecorderClient{Registry: reg}, ref); err != nil {
		note = "lineage lookup failed: " + err.Error()
	} else if ok {
		baseDigest, baseVersion = base, v
	}
	if baseDigest == "" && hasMapping && mapping.BaseDigest != "" {
		baseDigest, baseVersion = mapping.BaseDigest, firstNonEmpty(baseVersion, mapping.BaseVersion)
	}
	if baseDigest == "" {
		baseDigest = parsed.Digest
	}
	if version == "" {
		version = baseVersion
	}
	if version == "" && hasMapping {
		version = mapping.BaseVersion
	}

	exists, err := reg.Exists(ctx, ref)
	if err != nil {
		return resolvedSource{}, err
	}
	if exists {
		return resolvedSource{Ref: ref, BaseDigest: baseDigest, BaseVersion: version, Note: note}, nil
	}

	child, ok := a.chooseChild(ctx, reg, service, parsed.Repository, parsed.Digest, baseDigest, prefs)
	if !ok {
		return resolvedSource{}, fmt.Errorf("%s: source %s is missing and no patched child was found", service, ref)
	}
	return resolvedSource{
		Ref:         child.Ref,
		BaseDigest:  firstNonEmpty(child.BaseDigest, baseDigest),
		BaseVersion: firstNonEmpty(version, child.BaseVersion),
		Note:        joinNote(note, "parent missing; using oldest patched child "+shortRef(child.Ref)),
	}, nil
}

type childCandidate struct {
	Ref         string
	BaseDigest  string
	BaseVersion string
	PatchedAt   string
}

// chooseChild returns the oldest surviving patched descendant of a parent.
// Parent-side referrers die with the parent, so the lineage index is consulted
// as well; that is what keeps the fallback working after a base is deleted.
func (a *App) chooseChild(ctx context.Context, reg patchRegistry, service, repository, parentDigest, baseDigest string, prefs *catalog.File) (childCandidate, bool) {
	reader := &lineage.RecorderClient{Registry: reg}
	candidates := make([]childCandidate, 0)
	if children, err := reader.Children(ctx, repository+"@"+parentDigest); err == nil {
		for _, c := range children {
			candidates = append(candidates, childCandidate{
				Ref:         repository + "@" + c.Digest,
				BaseDigest:  c.BaseDigest,
				BaseVersion: c.BaseVersion,
				PatchedAt:   c.PatchedAt,
			})
		}
	}
	// Parent-side referrers die with the parent, so also ask the lineage log
	// for every child of the base image. This is what keeps fallback working
	// after the original base image is deleted from the registry.
	if baseDigest != "" {
		if children, err := reader.ChildrenForBase(ctx, repository, baseDigest); err == nil {
			for _, c := range children {
				candidates = append(candidates, childCandidate{
					Ref:         repository + "@" + c.Digest,
					BaseDigest:  c.BaseDigest,
					BaseVersion: c.BaseVersion,
					PatchedAt:   c.PatchedAt,
				})
			}
		}
	}
	if svc, ok := prefs.Services[service]; ok {
		for _, m := range svc.Mappings {
			if !sameDigest(m.Source, parentDigest) && !sameDigest(m.BaseDigest, parentDigest) && !sameDigest(m.Source, repository+"@"+parentDigest) {
				continue
			}
			candidates = append(candidates, childCandidate{
				Ref:         m.Target,
				BaseDigest:  m.BaseDigest,
				BaseVersion: m.BaseVersion,
				PatchedAt:   m.PatchedAt,
			})
		}
	}

	seen := map[string]bool{}
	valid := make([]childCandidate, 0, len(candidates))
	for _, c := range candidates {
		parsed, err := image.Parse(c.Ref)
		if err != nil || parsed.Digest == "" || seen[parsed.Digest] {
			continue
		}
		exists, err := reg.Exists(ctx, c.Ref)
		if err != nil || !exists {
			continue
		}
		seen[parsed.Digest] = true
		valid = append(valid, c)
	}
	if len(valid) == 0 {
		return childCandidate{}, false
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return valid[i].PatchedAt < valid[j].PatchedAt
	})
	return valid[0], true
}

func sameDigest(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, err := image.Parse(a)
	if err != nil || ra.Digest == "" {
		return false
	}
	rb, err := image.Parse(b)
	if err != nil || rb.Digest == "" {
		return false
	}
	return ra.Digest == rb.Digest
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func joinNote(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

func (a *App) printPatchResults(results []patch.Result) {
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		state := "patched"
		note := r.Reason
		switch {
		case r.Err != nil:
			state = "failed"
			note = r.Err.Error()
		case r.Skipped:
			state = "skipped"
		case r.TargetDigest == "":
			state = "planned"
		}
		rows = append(rows, []string{r.Service, shortRef(r.Source), shortRef(r.Target), state, note})
	}
	a.writeTable([]string{"SERVICE", "SOURCE", "TARGET", "STATE", "NOTE"}, rows)
}

func (a *App) writePatchMapping(override string, results []patch.Result) error {
	prefs, path, err := a.loadPreferences(override)
	if err != nil {
		return err
	}
	changed := 0
	for _, r := range results {
		if r.Err != nil || r.TargetDigest == "" {
			continue
		}
		base := r.BaseDigest
		if base == "" {
			base = r.SourceDigest
		}
		patchedAt := r.PatchedAt
		if patchedAt == "" {
			patchedAt = time.Now().UTC().Format(time.RFC3339)
		}
		if err := prefs.SetMapping(r.Service, catalog.Mapping{
			Source:      r.SourceRef(),
			Target:      r.TargetRef(),
			BaseDigest:  base,
			BaseVersion: r.BaseVersion,
			PatchedAt:   patchedAt,
			Note:        "patched",
		}); err != nil {
			return err
		}
		changed++
	}
	if changed == 0 {
		fmt.Fprintln(a.Out, "preferences unchanged")
		return nil
	}
	if err := prefs.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "updated %s (%d mapping(s))\n", path, changed)
	return nil
}

func patchResultsError(results []patch.Result) error {
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d image(s) failed", failed)
	}
	return nil
}
