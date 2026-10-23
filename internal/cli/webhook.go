package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/config"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/webhook"
)

func (a *App) webhookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage the Deployment mutating admission webhook",
	}
	cmd.AddCommand(
		a.webhookServeCmd(),
		a.webhookInstallCmd(),
		a.webhookUninstallCmd(),
		a.webhookStatusCmd(),
	)
	return cmd
}

func (a *App) webhookServeCmd() *cobra.Command {
	var addr, certDir, preferences string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the webhook HTTPS server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			dir := certDir
			if dir == "" {
				dir = a.cfg.Webhook.CertDir
			}
			certs, err := webhook.EnsureCerts(dir, a.cfg.Webhook.Host, false)
			if err != nil {
				return err
			}
			prefsPath := preferences
			if prefsPath == "" {
				prefsPath = a.cfg.Preferences
			}
			listen := addr
			if listen == "" {
				listen = fmt.Sprintf(":%d", a.cfg.Webhook.Port)
			}

			srv := &webhook.Server{
				Addr:     listen,
				CertFile: certs.CertFile,
				KeyFile:  certs.KeyFile,
				Mutator: &webhook.Mutator{
					Registry: a.registryClient(),
					LoadPreferences: func() (*catalog.File, error) {
						return catalog.Load(prefsPath)
					},
				},
			}
			fmt.Fprintf(a.Out, "webhook listening on %s (certs %s, preferences %s)\n", listen, dir, prefsPath)
			return srv.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "listen address (default :<webhook.port>)")
	cmd.Flags().StringVar(&certDir, "cert-dir", "", "certificate directory (default from config)")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file (default from config)")
	return cmd
}

func (a *App) webhookInstallCmd() *cobra.Command {
	var clusterID, host, certDir, failurePolicy, namespaces string
	var port int
	var forceCerts bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Create or update the MutatingWebhookConfiguration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}

			dir := certDir
			if dir == "" {
				dir = a.cfg.Webhook.CertDir
			}
			webhookHost := host
			if webhookHost == "" {
				webhookHost = a.cfg.Webhook.Host
			}
			webhookPort := port
			if webhookPort == 0 {
				webhookPort = a.cfg.Webhook.Port
			}
			policy := failurePolicy
			if policy == "" {
				policy = a.cfg.Webhook.FailurePolicy
			}
			selectedNamespaces := csvList(namespaces)

			certs, err := webhook.EnsureCerts(dir, webhookHost, forceCerts)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("https://%s:%d/mutate", webhookHost, webhookPort)
			if err := webhook.Install(cmd.Context(), client, webhook.InstallOptions{
				URL:            url,
				CABundle:       certs.CACert,
				Namespaces:     selectedNamespaces,
				FailurePolicy:  policy,
				TimeoutSeconds: 5,
			}); err != nil {
				return err
			}
			watched := "all namespaces"
			if len(selectedNamespaces) > 0 {
				watched = strings.Join(selectedNamespaces, ",")
			}
			fmt.Fprintf(a.Out, "installed %s in %s\n  url: %s\n  namespaces: %s\n  failurePolicy: %s\n",
				webhook.ConfigurationName, cl.ID, url, watched, policy)
			fmt.Fprintln(a.Out, "run `caretaker webhook serve` and make sure the API server can reach that URL")
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id to install into (default: first configured)")
	cmd.Flags().StringVar(&host, "host", "", "host the API server uses to reach the webhook (default from config)")
	cmd.Flags().IntVar(&port, "port", 0, "webhook port (default from config)")
	cmd.Flags().StringVar(&certDir, "cert-dir", "", "certificate directory (default from config)")
	cmd.Flags().StringVar(&failurePolicy, "failure-policy", "", "Ignore or Fail (default from config)")
	cmd.Flags().StringVar(&namespaces, "namespaces", "", "comma-separated namespaces to restrict to (default: all namespaces)")
	cmd.Flags().BoolVar(&forceCerts, "force-certs", false, "regenerate the CA and server certificate")
	return cmd
}

func (a *App) webhookUninstallCmd() *cobra.Command {
	var clusterID string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Delete the MutatingWebhookConfiguration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			if err := webhook.Uninstall(cmd.Context(), client); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "uninstalled %s from %s\n", webhook.ConfigurationName, cl.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	return cmd
}

func (a *App) webhookStatusCmd() *cobra.Command {
	var clusterID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the installed webhook configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			cfg, err := webhook.Status(cmd.Context(), client)
			if err != nil {
				return err
			}
			if cfg == nil {
				fmt.Fprintf(a.Out, "%s is not installed in %s\n", webhook.ConfigurationName, cl.ID)
				return nil
			}
			for _, hook := range cfg.Webhooks {
				url := "-"
				if hook.ClientConfig.URL != nil {
					url = *hook.ClientConfig.URL
				}
				policy := "-"
				if hook.FailurePolicy != nil {
					policy = string(*hook.FailurePolicy)
				}
				namespaces := "-"
				if hook.NamespaceSelector != nil {
					var values []string
					for _, expr := range hook.NamespaceSelector.MatchExpressions {
						values = append(values, expr.Values...)
					}
					if len(values) > 0 {
						namespaces = strings.Join(values, ",")
					}
				}
				a.writeTable(
					[]string{"CLUSTER", "WEBHOOK", "URL", "NAMESPACES", "FAILURE", "CA BYTES"},
					[][]string{{
						cl.ID, hook.Name, url, namespaces, policy,
						fmt.Sprintf("%d", len(hook.ClientConfig.CABundle)),
					}},
				)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	return cmd
}

func (a *App) webhookClient(clusterID string) (config.Cluster, kubernetes.Interface, error) {
	var cl config.Cluster
	if clusterID == "" {
		if len(a.cfg.Clusters) == 0 {
			return cl, nil, errors.New("no clusters configured")
		}
		cl = a.cfg.Clusters[0]
	} else {
		found, ok := a.cfg.ClusterByID(clusterID, a.Namespace)
		if !ok {
			return cl, nil, fmt.Errorf("unknown cluster %q", clusterID)
		}
		cl = found
	}
	restCfg, err := kube.NewRESTConfig(cl.Context, a.kubeconfigPath(cl))
	if err != nil {
		return cl, nil, err
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return cl, nil, err
	}
	return cl, client, nil
}

func csvList(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
