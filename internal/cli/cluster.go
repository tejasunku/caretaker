package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/kube"
)

func (a *App) clustersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clusters",
		Short: "List configured clusters and live deployment counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rows := make([][]string, 0, len(a.cfg.Clusters))
			for _, cl := range a.cfg.Clusters {
				state := "-"
				client, err := a.clusterClient(cl)
				if err != nil {
					state = "config-error"
				} else if deployments, err := client.ListDeployments(ctx); err != nil {
					state = "unreachable"
				} else {
					state = fmt.Sprintf("%d", len(deployments))
				}
				rows = append(rows, []string{cl.ID, cl.Stage, cl.Context, cl.Namespace, state})
			}
			a.writeTable([]string{"CLUSTER", "STAGE", "CONTEXT", "NAMESPACE", "DEPLOYMENTS"}, rows)
			return nil
		},
	}
}

func (a *App) statusCmd() *cobra.Command {
	var clustersSel, servicesSel, preferences string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show running images, LKG, and preferred patches",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			prefs, _, prefsErr := a.loadPreferences(preferences)
			if prefsErr != nil {
				return prefsErr
			}
			clients, err := a.clusterClients(clustersSel)
			if err != nil {
				return err
			}
			services, err := a.cfg.ResolveServices(servicesSel)
			if err != nil {
				return err
			}
			wanted := toSet(services)

			rows := make([][]string, 0)
			for _, cl := range clients {
				deployments, err := cl.ListDeployments(ctx)
				if err != nil {
					rows = append(rows, []string{cl.ID(), "-", "-", "-", "-", "unreachable"})
					continue
				}
				for _, d := range deployments {
					if !wanted[d.Service] {
						continue
					}
					marker := "-"
					preferred := "-"
					if target, ok := a.updater().Preferred(ctx, d, prefs); ok {
						preferred = shortRef(target)
						if sameImage(target, d.Image) {
							marker = "ok"
						} else {
							marker = "STALE"
						}
					}
					rows = append(rows, []string{
						cl.ID(),
						d.Service,
						shortRef(d.Image),
						shortRef(d.Annotations[kube.AnnotationLKG]),
						preferred,
						marker,
					})
				}
			}
			a.writeTable([]string{"CLUSTER", "SERVICE", "IMAGE", "LKG", "PREFERRED", "STATE"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&clustersSel, "clusters", "all", "comma-separated cluster ids, or all")
	cmd.Flags().StringVar(&servicesSel, "services", "all", "comma-separated services, or all")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file (default from config)")
	return cmd
}

func (a *App) lkgCmd() *cobra.Command {
	var clustersSel, servicesSel string
	cmd := &cobra.Command{
		Use:   "lkg",
		Short: "Show last-known-good images recorded in each cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			clients, err := a.clusterClients(clustersSel)
			if err != nil {
				return err
			}
			services, err := a.cfg.ResolveServices(servicesSel)
			if err != nil {
				return err
			}
			wanted := toSet(services)
			entries, err := a.updater().ListLKG(ctx, clients)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				if !wanted[e.Service] {
					continue
				}
				rows = append(rows, []string{
					e.Cluster, e.Service, shortRef(e.Current), shortRef(e.LastKnownGood),
					shortRef(e.Previous), e.UpdatedAt,
				})
			}
			a.writeTable([]string{"CLUSTER", "SERVICE", "CURRENT", "LAST-KNOWN-GOOD", "PREVIOUS", "UPDATED"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&clustersSel, "clusters", "all", "comma-separated cluster ids, or all")
	cmd.Flags().StringVar(&servicesSel, "services", "all", "comma-separated services, or all")
	return cmd
}

func toSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}
