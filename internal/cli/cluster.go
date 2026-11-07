package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/tejasunku/caretaker/internal/config"
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
				version := "-"
				client, err := a.clusterClient(cl)
				if err != nil {
					state = "config-error"
				} else if deployments, err := client.ListDeployments(ctx); err != nil {
					state = "unreachable"
				} else {
					state = fmt.Sprintf("%d", len(deployments))
					if got, verr := a.clusterVersion(ctx, cl); verr == nil {
						version = got
					}
				}
				rows = append(rows, []string{cl.ID, cl.Stage, cl.Context, cl.Namespace, version, state})
			}
			a.writeTable([]string{"CLUSTER", "STAGE", "CONTEXT", "NAMESPACE", "K8S", "DEPLOYMENTS"}, rows)
			return nil
		},
	}
}

func (a *App) clusterVersion(ctx context.Context, cl config.Cluster) (string, error) {
	restCfg, err := kube.NewRESTConfig(cl.Context, a.kubeconfigPath(cl))
	if err != nil {
		return "", err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", err
	}
	info, err := cs.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

func kubernetesVersionAtLeast(version string, major, minor int) bool {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	gotMajor, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	gotMinor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor)
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
			u := a.updater()
			for _, cl := range clients {
				deployments, err := cl.ListDeployments(ctx)
				if err != nil {
					rows = append(rows, []string{cl.ID(), "-", "-", "-", "-", "unreachable", ""})
					continue
				}
				for _, d := range deployments {
					if !wanted[d.Service] {
						continue
					}
					target, resolved, issues, hasMappings := resolutionIssues(ctx, prefs, u, d)
					marker := "-"
					preferred := "-"
					note := "no mappings"
					if hasMappings {
						note = strings.Join(issues, "; ")
					}
					if resolved {
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
						note,
					})
				}
			}
			a.writeTable([]string{"CLUSTER", "SERVICE", "IMAGE", "LKG", "PREFERRED", "STATE", "NOTE"}, rows)
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
