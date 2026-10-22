package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/update"
)

func (a *App) updateCmd() *cobra.Command {
	var clustersSel, servicesSel, preferences, healthCmd string
	var maps []string
	var dryRun, noVerify, noRollback bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Hot-swap preferred patched images into clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			prefs, _, err := a.loadPreferences(preferences)
			if err != nil {
				return err
			}
			explicit, err := parseMaps(maps)
			if err != nil {
				return err
			}
			services, err := a.cfg.ResolveServices(servicesSel)
			if err != nil {
				return err
			}
			clients, err := a.clusterClients(clustersSel)
			if err != nil {
				return err
			}

			opts := update.Options{
				DryRun:        dryRun,
				Verify:        !noVerify,
				AutoRollback:  !noRollback,
				Timeout:       timeout,
				HealthCommand: healthCmd,
				Explicit:      explicit,
			}
			results := a.updater().UpdateAll(ctx, clients, services, prefs, opts)
			a.printResults(results)
			return resultsError(results)
		},
	}

	cmd.Flags().StringVar(&clustersSel, "clusters", "all", "comma-separated cluster ids, or all")
	cmd.Flags().StringVar(&servicesSel, "services", "all", "comma-separated services, or all")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file (default from config)")
	cmd.Flags().StringArrayVar(&maps, "map", nil, "explicit mapping service=image (repeatable)")
	cmd.Flags().StringVar(&healthCmd, "health-cmd", "", "shell command run after rollout (placeholders {cluster} {namespace} {service})")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "rollout/verification timeout")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "skip rollout/health verification")
	cmd.Flags().BoolVar(&noRollback, "no-rollback", false, "do not roll back automatically when verification fails")
	return cmd
}

func (a *App) rollbackCmd() *cobra.Command {
	var clustersSel, servicesSel, imageRef string
	var dryRun, fromLKG bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Return services to their last-known-good image (or an explicit image)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			services, err := a.cfg.ResolveServices(servicesSel)
			if err != nil {
				return err
			}
			clients, err := a.clusterClients(clustersSel)
			if err != nil {
				return err
			}
			opts := update.Options{DryRun: dryRun, Timeout: timeout, FromLKG: fromLKG}
			results := a.updater().RollbackAll(ctx, clients, services, imageRef, opts)
			a.printResults(results)
			return resultsError(results)
		},
	}

	cmd.Flags().StringVar(&clustersSel, "clusters", "all", "comma-separated cluster ids, or all")
	cmd.Flags().StringVar(&servicesSel, "services", "all", "comma-separated services, or all")
	cmd.Flags().StringVar(&imageRef, "image", "", "explicit image to roll back to (default: recorded LKG)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "rollout timeout")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	cmd.Flags().BoolVar(&fromLKG, "from-lkg", false, "roll back to the recorded LKG instead of the previous image")
	return cmd
}
