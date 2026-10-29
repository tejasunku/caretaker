// Package cli wires cobra commands to caretaker's internal packages. Commands
// parse flags and print; decisions live elsewhere.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/config"
	"github.com/tejasunku/caretaker/internal/health"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/patch"
	"github.com/tejasunku/caretaker/internal/registry"
	"github.com/tejasunku/caretaker/internal/runner"
	"github.com/tejasunku/caretaker/internal/update"
)

// Version is injected at build time with -ldflags.
var Version = "dev"

// App holds global flags and shared clients.
type App struct {
	ConfigPath string
	Kubeconfig string
	Namespace  string
	AllowTags  bool
	Out        io.Writer
	Err        io.Writer

	cfg *config.Config
}

// New returns an App writing to stdout/stderr.
func New() *App {
	return &App{Out: os.Stdout, Err: os.Stderr}
}

// Execute runs the command tree.
func (a *App) Execute() error {
	return a.rootCmd().Execute()
}

func (a *App) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "caretaker",
		Short:         "Patch container images and stage them across clusters",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(a.ConfigPath)
			if err != nil {
				return err
			}
			a.cfg = cfg
			return nil
		},
	}
	root.PersistentFlags().StringVar(&a.ConfigPath, "config", "", "config file (default caretaker.yaml or $CARETAKER_CONFIG)")
	root.PersistentFlags().StringVar(&a.Kubeconfig, "kubeconfig", "", "kubeconfig path (default $CARETAKER_KUBECONFIG or ~/.kube/config)")
	root.PersistentFlags().StringVar(&a.Namespace, "namespace", "", "override the namespace for all clusters")
	root.PersistentFlags().BoolVar(&a.AllowTags, "allow-tags", false, "opt in to tag-based input and discovery")

	root.AddCommand(
		a.doctorCmd(),
		a.configCmd(),
		a.clustersCmd(),
		a.statusCmd(),
		a.lkgCmd(),
		a.patchCmd(),
		a.scanCmd(),
		a.updateCmd(),
		a.rollbackCmd(),
		a.mappingCmd(),
		a.lineageCmd(),
		a.webhookCmd(),
	)
	return root
}

func (a *App) allowTags() bool {
	return a.AllowTags || (a.cfg != nil && a.cfg.AllowTags)
}

func (a *App) registryClient() *registry.Client {
	return registry.New(a.cfg.Registry)
}

type patchOptions struct {
	Report      string
	DryRun      bool
	SkipIfClean bool
	Concurrency int
}

func (a *App) patcher(opts patchOptions) *patch.Patcher {
	report := opts.Report
	if report == "" {
		report = a.cfg.Patch.Report
	}
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	reg := a.registryClient()
	return &patch.Patcher{
		Runner:       runner.Exec{Stdout: a.Out, Stderr: a.Err},
		Copa:         a.cfg.Patch.Copa,
		Trivy:        a.cfg.Patch.Trivy,
		BuildkitAddr: a.cfg.Patch.BuildkitAddr,
		Platform:     a.cfg.Patch.Platform,
		TagSuffix:    a.cfg.Patch.TagSuffix,
		Report:       report,
		DryRun:       opts.DryRun,
		SkipIfClean:  opts.SkipIfClean,
		Concurrency:  concurrency,
		Now:          time.Now,
		Lineage:      &lineage.RecorderClient{Registry: reg},
	}
}

func (a *App) updater() *update.Updater {
	return &update.Updater{
		Registry:  a.registryClient(),
		Health:    health.Checker{Runner: runner.Exec{Stdout: a.Out, Stderr: a.Err}},
		Now:       time.Now,
		AllowTags: a.allowTags(),
	}
}

func (a *App) kubeconfigPath(cl config.Cluster) string {
	if cl.Kubeconfig != "" {
		return cl.Kubeconfig
	}
	if a.Kubeconfig != "" {
		return a.Kubeconfig
	}
	return a.cfg.Kubeconfig
}

func (a *App) clusterClient(cl config.Cluster) (kube.Cluster, error) {
	ns := cl.Namespace
	if a.Namespace != "" {
		ns = a.Namespace
	}
	return kube.NewClient(cl.ID, cl.Context, ns, a.kubeconfigPath(cl))
}

func (a *App) clusterClients(selector string) ([]kube.Cluster, error) {
	configured, err := a.cfg.ResolveClusters(selector, a.Namespace)
	if err != nil {
		return nil, err
	}
	out := make([]kube.Cluster, 0, len(configured))
	for _, cl := range configured {
		client, err := a.clusterClient(cl)
		if err != nil {
			return nil, err
		}
		out = append(out, client)
	}
	return out, nil
}

func (a *App) loadPreferences(override string) (*catalog.File, string, error) {
	path := override
	if path == "" {
		path = a.cfg.Preferences
	}
	file, err := catalog.Load(path)
	return file, path, err
}

func (a *App) writeTable(headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(a.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}

func (a *App) printResults(results []update.Result) {
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		note := r.Reason
		if r.Err != nil {
			note = r.Err.Error()
		}
		rows = append(rows, []string{r.Cluster, r.Service, r.Action, shortRef(r.From), shortRef(r.To), note})
	}
	a.writeTable([]string{"CLUSTER", "SERVICE", "ACTION", "FROM", "TO", "NOTE"}, rows)
}

func resultsError(results []update.Result) error {
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d operation(s) failed", failed)
	}
	return nil
}

func parseMaps(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		service, ref, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(service) == "" || strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("invalid --map %q (want service=image)", pair)
		}
		if _, err := image.Parse(strings.TrimSpace(ref)); err != nil {
			return nil, fmt.Errorf("invalid --map %q: %w", pair, err)
		}
		out[strings.TrimSpace(service)] = strings.TrimSpace(ref)
	}
	return out, nil
}

func shortRef(ref string) string {
	if ref == "" {
		return "-"
	}
	if i := strings.Index(ref, "@sha256:"); i >= 0 {
		digest := ref[i+len("@sha256:"):]
		if len(digest) > 12 {
			digest = digest[:12]
		}
		return ref[:i] + "@sha256:" + digest
	}
	if len(ref) > 80 {
		return ref[:77] + "..."
	}
	return ref
}

func sameImage(a, b string) bool {
	ra, err := image.Parse(a)
	if err != nil {
		return a == b
	}
	rb, err := image.Parse(b)
	if err != nil {
		return a == b
	}
	return ra.SameImage(rb)
}

func requireService(service string) error {
	if strings.TrimSpace(service) == "" {
		return errors.New("--service is required")
	}
	return nil
}

func errString(message string) error { return errors.New(message) }
