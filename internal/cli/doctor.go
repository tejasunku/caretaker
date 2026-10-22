package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
		Short: "Check config, tools, registry, and cluster connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			rows := [][]string{}
			failures := 0

			add := func(check, state, detail string) {
				rows = append(rows, []string{check, state, detail})
				if state != "ok" {
					failures++
				}
			}

			add("config", "ok", a.cfg.Path)
			add("preferences", checkFile(a.cfg.Preferences), a.cfg.Preferences)
			add("copa", checkExecutable(a.cfg.Patch.Copa), a.cfg.Patch.Copa)
			add("trivy", checkExecutable(a.cfg.Patch.Trivy), a.cfg.Patch.Trivy)

			reg := a.registryClient()
			if repos, err := reg.Catalog(ctx); err != nil {
				add("registry", "error", err.Error())
			} else {
				add("registry", "ok", fmt.Sprintf("%s (%d repositories)", a.cfg.Registry.URL, len(repos)))
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
			}

			a.writeTable([]string{"CHECK", "STATE", "DETAIL"}, rows)
			if failures > 0 {
				return fmt.Errorf("%d check(s) failed", failures)
			}
			return nil
		},
	}
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
		return "missing"
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "not-executable"
	}
	return "ok"
}
