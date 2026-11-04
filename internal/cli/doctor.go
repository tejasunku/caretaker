package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/update"
)

func (a *App) configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Configuration commands"}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := *a.cfg
			if c.Registry.Password != "" {
				c.Registry.Password = "***"
			}
			data, err := yaml.Marshal(&c)
			if err != nil {
				return err
			}
			_, err = a.Out.Write(data)
			return err
		},
	})
	return cmd
}

func (a *App) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config, tools, registry, mappings, and cluster state",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rows := [][]string{}
			failures := 0

			// "warn" is an anomaly worth displaying but not a failure: doctor
			// reports problems, it does not gate on them.
			add := func(check, state, detail string) {
				rows = append(rows, []string{check, state, detail})
				if state != "ok" && state != "warn" {
					failures++
				}
			}

			add("config", "ok", a.cfg.Path)

			var prefs *catalog.File
			if state := checkFile(a.cfg.Preferences); state != "ok" {
				add("preferences", state, a.cfg.Preferences)
			} else if loaded, err := catalog.Load(a.cfg.Preferences); err != nil {
				// Schema errors (legacy source-keyed files, wrong kind) fail loudly.
				add("preferences", "error", err.Error())
			} else {
				prefs = loaded
				add("preferences", "ok", fmt.Sprintf("%s (%d services)", a.cfg.Preferences, len(prefs.Services)))
			}

			add("copa", checkExecutable(a.cfg.Patch.Copa), a.cfg.Patch.Copa)
			add("trivy", checkExecutable(a.cfg.Patch.Trivy), a.cfg.Patch.Trivy)

			reg := a.registryClient()
			if repos, err := reg.Catalog(ctx); err != nil {
				add("registry", "error", err.Error())
			} else {
				add("registry", "ok", fmt.Sprintf("%s (%d repositories)", a.cfg.Registry.URL, len(repos)))
			}

			// Every mapped target must still exist in the registry, or an
			// update would swap a Deployment onto an image that cannot pull.
			if prefs != nil {
				for _, name := range prefs.ServiceNames() {
					mappings := prefs.Services[name].Mappings
					if len(mappings) == 0 {
						continue
					}
					var missing []string
					for _, m := range mappings {
						exists, err := reg.Exists(ctx, m.Target)
						if err != nil || !exists {
							missing = append(missing, shortRef(m.Target))
						}
					}
					if len(missing) > 0 {
						add("map/"+name, "error", fmt.Sprintf("%d mapping(s); missing target(s): %s", len(mappings), strings.Join(missing, ", ")))
						continue
					}
					add("map/"+name, "ok", fmt.Sprintf("%d mapping(s), targets exist", len(mappings)))
				}
			}

			resolver := &update.Updater{
				Registry:  reg,
				Lineage:   &lineage.RecorderClient{Registry: reg},
				AllowTags: a.allowTags(),
			}
			for _, cl := range a.cfg.Clusters {
				client, err := a.clusterClient(cl)
				if err != nil {
					add("cluster/"+cl.ID, "error", err.Error())
					continue
				}
				deployments, err := client.ListDeployments(ctx)
				if err != nil {
					add("cluster/"+cl.ID, "error", err.Error())
					continue
				}
				add("cluster/"+cl.ID, "ok", fmt.Sprintf("%s/%s (%d deployments)", cl.Context, cl.Namespace, len(deployments)))
				if prefs == nil {
					continue
				}
				sort.Slice(deployments, func(i, j int) bool {
					return deployments[i].Service < deployments[j].Service
				})
				for _, d := range deployments {
					if state, detail, ok := doctorResolution(ctx, prefs, resolver, d); ok {
						add("deploy/"+cl.ID+"/"+d.Service, state, detail)
					}
				}
			}

			a.writeTable([]string{"CHECK", "STATE", "DETAIL"}, rows)
			if failures > 0 {
				return fmt.Errorf("%d check(s) failed", failures)
			}
			return nil
		},
	}
}

// resolutionIssues reports how a running image resolves against the preference
// map plus any anomalies: an unresolved image when the service has mappings,
// and a base-digest annotation that names no feature build. An empty issues
// slice means the row is clean. ok=false means the service has no mappings at
// all, so there is nothing to check.
func resolutionIssues(ctx context.Context, prefs *catalog.File, u *update.Updater, d kube.Deployment) (target string, resolved bool, issues []string, ok bool) {
	if prefs == nil || len(prefs.Services[d.Service].Mappings) == 0 {
		return "", false, nil, false
	}
	target, resolved = u.Preferred(ctx, d, prefs)
	if !resolved {
		issues = append(issues, "no preferred mapping (update falls back to the lineage index)")
	}
	if ann := d.Annotations[kube.AnnotationBaseDigest]; ann != "" {
		base := ann
		if !strings.Contains(ann, "/") && d.Ref.Repository != "" {
			base = d.Ref.Repository + "@" + ann
		}
		if _, found := prefs.Lookup(d.Service, base); !found {
			issues = append(issues, "base-digest "+shortRef(base)+" has no feature-build mapping")
		}
	}
	return target, resolved, issues, true
}

// doctorResolution checks one Deployment against the preference map. It returns
// ok=false when the service has no mappings, because there is nothing to
// resolve.
func doctorResolution(ctx context.Context, prefs *catalog.File, u *update.Updater, d kube.Deployment) (string, string, bool) {
	target, _, issues, ok := resolutionIssues(ctx, prefs, u, d)
	if !ok {
		return "", "", false
	}
	if len(issues) == 0 {
		return "ok", "prefers " + shortRef(target), true
	}
	return "warn", strings.Join(issues, "; "), true
}

func checkFile(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "missing"
		}
		return "error"
	}
	if info.IsDir() {
		return "error"
	}
	return "ok"
}

func checkExecutable(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "missing"
		}
		return "error"
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "not-executable"
	}
	return "ok"
}
