package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/image"
)

func (a *App) mappingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mapping",
		Short: "Inspect and update preferred-patch mappings",
	}
	cmd.AddCommand(a.mappingShowCmd(), a.mappingSetCmd(), a.mappingMergeCmd())
	return cmd
}

func (a *App) mappingShowCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the preferences file",
		RunE: func(cmd *cobra.Command, args []string) error {
			prefs, _, err := a.loadPreferences(file)
			if err != nil {
				return err
			}
			return prefs.Encode(a.Out)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "preferences file (default from config)")
	return cmd
}

func (a *App) mappingSetCmd() *cobra.Command {
	var file, service, base, target, note, baseVersion string
	var aliases []string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Add or replace one mapping",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireService(service); err != nil {
				return err
			}
			if base == "" || target == "" {
				return errString("--base and --target are required")
			}
			prefs, path, err := a.loadPreferences(file)
			if err != nil {
				return err
			}
			baseRef, err := a.resolveRef(cmd.Context(), base)
			if err != nil {
				return err
			}
			targetRef, err := a.resolveRef(cmd.Context(), target)
			if err != nil {
				return err
			}
			existing, hadExisting := prefs.MappingForBase(service, baseRef)
			m := catalog.Mapping{
				Base:        baseRef,
				Target:      targetRef,
				BaseVersion: baseVersion,
				Note:        note,
			}
			if aliases != nil {
				m.Aliases = aliases
			}
			if err := prefs.SetMapping(service, m); err != nil {
				return err
			}
			// The target changed by hand: the old patchedAt timestamp no
			// longer describes the image the mapping points at.
			if hadExisting && existing.Target != targetRef {
				svc := prefs.Services[service]
				for i := range svc.Mappings {
					if svc.Mappings[i].Base == baseRef {
						svc.Mappings[i].PatchedAt = ""
					}
				}
				prefs.Services[service] = svc
			}
			if err := prefs.Save(path); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "updated %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "preferences file (default from config)")
	cmd.Flags().StringVar(&service, "service", "", "service name (required)")
	cmd.Flags().StringVar(&base, "base", "", "feature build image digest (required; tags need --allow-tags)")
	cmd.Flags().StringVar(&target, "target", "", "preferred patched image digest (required; tags need --allow-tags)")
	cmd.Flags().StringVar(&note, "note", "", "optional note (default: preserve existing)")
	cmd.Flags().StringVar(&baseVersion, "base-version", "", "base version label, e.g. 1.0 (default: preserve existing)")
	cmd.Flags().StringSliceVar(&aliases, "alias", nil, "older patched digest of this feature build (repeatable; replaces the alias list)")
	return cmd
}

// resolveRef turns a tag reference into a digest-only reference so preference
// mappings never depend on mutable tags. Tags require --allow-tags.
func (a *App) resolveRef(ctx context.Context, ref string) (string, error) {
	if !strings.Contains(ref, "/") {
		return "", fmt.Errorf("reference %q must include a repository", ref)
	}
	parsed, err := image.Parse(ref)
	if err != nil {
		return "", err
	}
	if parsed.Digest != "" {
		return parsed.String(), nil
	}
	if !a.allowTags() {
		return "", fmt.Errorf("reference %q has no digest; caretaker is digest-first (use --allow-tags for tag input)", ref)
	}
	digest, err := a.registryClient().Resolve(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	return parsed.Repository + "@" + digest, nil
}

func (a *App) mappingMergeCmd() *cobra.Command {
	var file, updates, out string
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge an update file into an existing preferences file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if updates == "" {
				return errString("--updates is required")
			}
			base, path, err := a.loadPreferences(file)
			if err != nil {
				return err
			}
			patchFile, err := catalog.Load(updates)
			if err != nil {
				return err
			}
			if err := base.Merge(patchFile); err != nil {
				return err
			}
			target := out
			if target == "" {
				target = path
			}
			if err := base.Save(target); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "wrote %s\n", target)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "base preferences file (default from config)")
	cmd.Flags().StringVar(&updates, "updates", "", "preferences file with updates (required)")
	cmd.Flags().StringVar(&out, "out", "", "output file (default: overwrite --file)")
	return cmd
}
