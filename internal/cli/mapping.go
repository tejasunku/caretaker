package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/catalog"
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
	var file, service, source, target, note string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Add or replace one mapping",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireService(service); err != nil {
				return err
			}
			if source == "" || target == "" {
				return errString("--source and --target are required")
			}
			prefs, path, err := a.loadPreferences(file)
			if err != nil {
				return err
			}
			if err := prefs.Set(service, source, target, note); err != nil {
				return err
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
	cmd.Flags().StringVar(&source, "source", "", "source image or tag (required)")
	cmd.Flags().StringVar(&target, "target", "", "preferred patched image (required)")
	cmd.Flags().StringVar(&note, "note", "", "optional note")
	return cmd
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
			base.Merge(patchFile)
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
