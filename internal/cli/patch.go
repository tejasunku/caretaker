package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/patch"
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
	var service, source, tag, report, preferences string
	var writeMapping, force, dryRun bool
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Patch one service image",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireService(service); err != nil {
				return err
			}
			ctx := cmd.Context()
			reg := a.registryClient()
			repository := a.cfg.Repository(service)

			if source == "" {
				tags, err := reg.Tags(ctx, repository)
				if err != nil {
					return err
				}
				base, ok := image.NewestTag(tags, false)
				if !ok {
					return fmt.Errorf("no versioned tags in %s", repository)
				}
				source = repository + ":" + base
			}
			parsed, err := image.Parse(source)
			if err != nil {
				return err
			}
			if parsed.Tag == "" {
				return fmt.Errorf("--source must include a tag, got %q", source)
			}
			targetTag := tag
			if targetTag == "" {
				targetTag = image.PatchedTag(parsed.Tag, a.cfg.Patch.TagSuffix, time.Now())
			}

			res, err := a.patcher(report, dryRun).PatchImage(ctx, reg, service, source, targetTag, force)
			if err != nil {
				return err
			}
			a.printPatchResults([]patch.Result{res})
			if writeMapping {
				return a.writePatchMapping(preferences, []patch.Result{res})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&service, "service", "", "service name (required)")
	cmd.Flags().StringVar(&source, "source", "", "source image (default: newest base tag)")
	cmd.Flags().StringVar(&tag, "tag", "", "target tag (default: <source-tag>-patched-<today>)")
	cmd.Flags().StringVar(&report, "report", "", "vulnerability report file or directory for Copa")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file to update")
	cmd.Flags().BoolVar(&writeMapping, "write-mapping", false, "record the result in the preferences file")
	cmd.Flags().BoolVar(&force, "force", false, "re-patch even if the target tag exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func (a *App) patchAllCmd() *cobra.Command {
	var servicesSel, report, preferences string
	var writeMapping, force, dryRun bool
	cmd := &cobra.Command{
		Use:   "all",
		Short: "Patch every selected service image",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			services, err := a.cfg.ResolveServices(servicesSel)
			if err != nil {
				return err
			}
			results := a.patcher(report, dryRun).PatchAll(ctx, a.registryClient(), services, a.cfg.Repository, force)
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
	cmd.Flags().StringVar(&report, "report", "", "vulnerability report file or directory for Copa")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file to update")
	cmd.Flags().BoolVar(&writeMapping, "write-mapping", false, "record results in the preferences file")
	cmd.Flags().BoolVar(&force, "force", false, "re-patch even if target tags exist")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
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
		if err := prefs.Set(r.Service, r.SourceRef(), r.TargetRef(), "patched"); err != nil {
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
