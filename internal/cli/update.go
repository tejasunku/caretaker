package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/update"
)

func (a *App) updateCmd() *cobra.Command {
	var clustersSel, servicesSel, preferences, healthCmd string
	var maps []string
	var dryRun, noVerify, noFallback, noRollback, force bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:     "set-target",
		Aliases: []string{"update"},
		Short:   "Set the desired image for services",
		Long: `Set the desired image for each service. This is the normal path: the
preference map, admission webhook, and health verification all apply.

A target can be newer or older than the running image. Choosing an older image
is still a set-target operation; it is not a rollback.

Use restore instead when you need a break-glass return to a previous image that
bypasses admission.`,
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
				AutoFallback:  !noFallback && !noRollback,
				Force:         force,
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
	cmd.Flags().BoolVar(&noFallback, "no-fallback", false, "leave the new image in place when verification fails")
	cmd.Flags().BoolVar(&noRollback, "no-rollback", false, "deprecated alias for --no-fallback")
	_ = cmd.Flags().MarkDeprecated("no-rollback", "use --no-fallback")
	cmd.Flags().BoolVar(&force, "force", false, "bypass admission by setting caretaker.io/skip-webhook=true")
	return cmd
}

func (a *App) rollbackCmd() *cobra.Command {
	var clustersSel, servicesSel, imageRef string
	var dryRun, fromLKG, lkg bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:     "restore",
		Aliases: []string{"rollback"},
		Short:   "Restore a previous or last-known-good image (break-glass)",
		Long: `Restore a service to the image it replaced, the recorded last-known-good
image, or an explicit image.

This is a break-glass operation: it sets caretaker.io/skip-webhook=true before
the swap, so the admission webhook cannot immediately undo it. To choose an
older image as the desired state while keeping preferences and admission in
effect, use set-target instead.`,
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
			opts := update.Options{DryRun: dryRun, Timeout: timeout, FromLKG: fromLKG || lkg}
			results := a.updater().RollbackAll(ctx, clients, services, imageRef, opts)
			a.printResults(results)
			return resultsError(results)
		},
	}

	cmd.Flags().StringVar(&clustersSel, "clusters", "all", "comma-separated cluster ids, or all")
	cmd.Flags().StringVar(&servicesSel, "services", "all", "comma-separated services, or all")
	cmd.Flags().StringVar(&imageRef, "image", "", "explicit image to restore (default: recorded previous image)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "rollout timeout")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	cmd.Flags().BoolVar(&lkg, "lkg", false, "restore the recorded last-known-good image instead of the previous image")
	cmd.Flags().BoolVar(&fromLKG, "from-lkg", false, "deprecated alias for --lkg")
	_ = cmd.Flags().MarkDeprecated("from-lkg", "use --lkg")
	return cmd
}
