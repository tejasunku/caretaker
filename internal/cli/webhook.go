package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
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
		a.webhookDeployCmd(),
		a.webhookUndeployCmd(),
		a.webhookMapCmd(),
		a.webhookPrefsCmd(),
		a.webhookUninstallCmd(),
		a.webhookStatusCmd(),
	)
	return cmd
}

func (a *App) webhookServeCmd() *cobra.Command {
	var addr, certDir, preferences string
	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Run the webhook HTTPS server (in-cluster entrypoint)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			dir := certDir
			if dir == "" {
				dir = a.cfg.Webhook.CertDir
			}
			certs, err := webhook.LoadCerts(dir)
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

func (a *App) webhookDeployCmd() *cobra.Command {
	var clusterID, namespace, serviceName, image, preferences, certDir, failurePolicy, namespaces string
	var registryURL, registryNamespace string
	var registryInsecure bool
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the webhook in-cluster with a preferences ConfigMap",
		Long: "Deploy installs the webhook as a Deployment and Service, stores the preferences\n" +
			"document in a ConfigMap, generates TLS material, and registers the\n" +
			"MutatingWebhookConfiguration against the Service. The API server never calls\n" +
			"back to this workstation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			prefsPath := preferences
			if prefsPath == "" {
				prefsPath = a.cfg.Preferences
			}
			data, err := os.ReadFile(prefsPath)
			if err != nil {
				return fmt.Errorf("read preferences: %w", err)
			}
			if _, err := catalog.Decode(data, prefsPath); err != nil {
				return err
			}
			policy := failurePolicy
			if policy == "" {
				policy = a.cfg.Webhook.FailurePolicy
			}
			dir := certDir
			if dir == "" {
				dir = a.cfg.Webhook.CertDir
			}
			opts := webhook.DeployOptions{
				Namespace:         namespace,
				ServiceName:       serviceName,
				Image:             image,
				Preferences:       data,
				CertDir:           dir,
				RegistryURL:       firstNonEmptyCLI(registryURL, a.cfg.Registry.URL),
				RegistryNamespace: firstNonEmptyCLI(registryNamespace, a.cfg.Registry.Namespace),
				RegistryInsecure:  registryInsecure || a.cfg.Registry.Insecure,
				FailurePolicy:     policy,
				Namespaces:        csvList(namespaces),
			}
			if err := webhook.Deploy(cmd.Context(), client, opts); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "deployed %s in %s\n  namespace: %s\n  service: %s\n  image: %s\n  preferences: ConfigMap/%s\n",
				webhook.ConfigurationName, cl.ID,
				firstNonEmptyCLI(namespace, webhook.DefaultNamespace),
				firstNonEmptyCLI(serviceName, webhook.DefaultServiceName),
				image, webhook.DefaultConfigName)
			fmt.Fprintf(a.Out, "update preferences with `caretaker webhook prefs push --cluster %s`\n", cl.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace for the webhook (default caretaker-system)")
	cmd.Flags().StringVar(&serviceName, "service-name", "", "Service and Deployment name (default caretaker-webhook)")
	cmd.Flags().StringVar(&image, "image", "", "webhook image reference (required)")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file (default from config)")
	cmd.Flags().StringVar(&certDir, "cert-dir", "", "certificate directory (default from config)")
	cmd.Flags().StringVar(&failurePolicy, "failure-policy", "", "Ignore or Fail (default from config)")
	cmd.Flags().StringVar(&namespaces, "namespaces", "", "comma-separated namespaces to restrict to (default: all)")
	cmd.Flags().StringVar(&registryURL, "registry-url", "", "registry the webhook should use (default from config; in kind use the host gateway, e.g. 10.89.2.1:5000)")
	cmd.Flags().StringVar(&registryNamespace, "registry-namespace", "", "registry namespace (default from config)")
	cmd.Flags().BoolVar(&registryInsecure, "registry-insecure", false, "use plain HTTP for the registry")
	return cmd
}

func (a *App) webhookUndeployCmd() *cobra.Command {
	var clusterID, namespace, serviceName string
	cmd := &cobra.Command{
		Use:   "undeploy",
		Short: "Remove the in-cluster webhook Deployment, Service, ConfigMap, and Secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			if err := webhook.Undeploy(cmd.Context(), client, namespace, serviceName); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "removed in-cluster webhook from %s\n", cl.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default caretaker-system)")
	cmd.Flags().StringVar(&serviceName, "service-name", "", "Service and Deployment name (default caretaker-webhook)")
	return cmd
}

func (a *App) webhookPrefsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prefs",
		Short: "Manage the in-cluster preferences ConfigMap",
	}
	cmd.AddCommand(a.webhookPrefsPushCmd())
	return cmd
}

func (a *App) webhookPrefsPushCmd() *cobra.Command {
	var clusterID, namespace, name, file string
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Create or update the preferences ConfigMap from a local file",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			if file == "" {
				file = a.cfg.Preferences
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read preferences: %w", err)
			}
			if _, err := catalog.Decode(data, file); err != nil {
				return err
			}
			ns := firstNonEmptyCLI(namespace, webhook.DefaultNamespace)
			if err := webhook.UpsertPreferences(cmd.Context(), client, ns, name, data); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "wrote ConfigMap/%s in %s (from %s)\n", firstNonEmptyCLI(name, webhook.DefaultConfigName), ns, file)
			fmt.Fprintln(a.Out, "note: the mounted ConfigMap refreshes in pods within about a minute")
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace (default caretaker-system)")
	cmd.Flags().StringVar(&name, "name", "", "ConfigMap name (default caretaker-preferences)")
	cmd.Flags().StringVar(&file, "file", "", "preferences file (default from config)")
	return cmd
}

func firstNonEmptyCLI(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
			version := "-"
			if info, verr := client.Discovery().ServerVersion(); verr == nil {
				version = info.GitVersion
			}
			for _, hook := range cfg.Webhooks {
				endpoint := "-"
				if hook.ClientConfig.URL != nil {
					endpoint = *hook.ClientConfig.URL
				}
				if svc := hook.ClientConfig.Service; svc != nil {
					port := int32(443)
					if svc.Port != nil {
						port = *svc.Port
					}
					path := "/mutate"
					if svc.Path != nil {
						path = *svc.Path
					}
					endpoint = fmt.Sprintf("service://%s.%s:%d%s", svc.Name, svc.Namespace, port, path)
				}
				policy := "-"
				if hook.FailurePolicy != nil {
					policy = string(*hook.FailurePolicy)
				}
				namespaces := "all"
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
					[]string{"CLUSTER", "K8S", "WEBHOOK", "ENDPOINT", "NAMESPACES", "FAILURE", "CA BYTES"},
					[][]string{{
						cl.ID, version, hook.Name, endpoint, namespaces, policy,
						fmt.Sprintf("%d", len(hook.ClientConfig.CABundle)),
					}},
				)
			}
			if kubernetesVersionAtLeast(version, 1, 36) {
				fmt.Fprintln(a.Out, "MutatingAdmissionPolicy is available on this cluster (alternative to this webhook)")
			} else {
				fmt.Fprintf(a.Out, "MutatingAdmissionPolicy requires Kubernetes 1.36+ (this cluster is %s)\n", version)
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

func (a *App) webhookMapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "map",
		Short: "Manage the in-cluster MutatingAdmissionPolicy (no server, Kubernetes 1.36+)",
		Long: "The MAP mode installs a MutatingAdmissionPolicy plus a binding and a digest-\n" +
			"to-target ConfigMap. The API server evaluates the policy with CEL, so no\n" +
			"webhook server, image, certificate, or host process is involved.",
	}
	cmd.AddCommand(
		a.webhookMapInstallCmd(),
		a.webhookMapUninstallCmd(),
		a.webhookMapPrefsCmd(),
		a.webhookMapStatusCmd(),
	)
	return cmd
}

func (a *App) webhookMapInstallCmd() *cobra.Command {
	var clusterID, namespace, policyName, configName, preferences, failurePolicy, namespaces string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the MutatingAdmissionPolicy and its image-map ConfigMap",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			dyn, err := a.dynamicClient(cl)
			if err != nil {
				return err
			}
			prefsPath := firstNonEmptyCLI(preferences, a.cfg.Preferences)
			data, err := os.ReadFile(prefsPath)
			if err != nil {
				return fmt.Errorf("read preferences: %w", err)
			}
			policy := firstNonEmptyCLI(failurePolicy, a.cfg.Webhook.FailurePolicy)
			opts := webhook.PolicyOptions{
				Namespace:     namespace,
				PolicyName:    policyName,
				ConfigName:    configName,
				Preferences:   data,
				FailurePolicy: policy,
				Namespaces:    csvList(namespaces),
			}
			if err := webhook.InstallPolicy(cmd.Context(), client, dyn, opts); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "installed MutatingAdmissionPolicy/%s and binding in %s\n", firstNonEmptyCLI(policyName, webhook.DefaultPolicyName), cl.ID)
			fmt.Fprintf(a.Out, "image map ConfigMap/%s in %s\n", firstNonEmptyCLI(configName, webhook.DefaultImageMapName), firstNonEmptyCLI(namespace, webhook.DefaultNamespace))
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace for the image-map ConfigMap")
	cmd.Flags().StringVar(&policyName, "policy-name", "", "MutatingAdmissionPolicy/binding name")
	cmd.Flags().StringVar(&configName, "config-name", "", "image-map ConfigMap name")
	cmd.Flags().StringVar(&preferences, "preferences", "", "preferences file (default from config)")
	cmd.Flags().StringVar(&failurePolicy, "failure-policy", "", "Ignore or Fail (default from config)")
	cmd.Flags().StringVar(&namespaces, "namespaces", "", "comma-separated namespaces to enforce (default all)")
	return cmd
}

func (a *App) webhookMapUninstallCmd() *cobra.Command {
	var clusterID, namespace, policyName, configName string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the MutatingAdmissionPolicy, binding, and image-map ConfigMap",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			dyn, err := a.dynamicClient(cl)
			if err != nil {
				return err
			}
			opts := webhook.PolicyOptions{Namespace: namespace, PolicyName: policyName, ConfigName: configName}
			if err := webhook.UninstallPolicy(cmd.Context(), client, dyn, opts); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "removed MutatingAdmissionPolicy/%s from %s\n", firstNonEmptyCLI(policyName, webhook.DefaultPolicyName), cl.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace of the image-map ConfigMap")
	cmd.Flags().StringVar(&policyName, "policy-name", "", "MutatingAdmissionPolicy/binding name")
	cmd.Flags().StringVar(&configName, "config-name", "", "image-map ConfigMap name")
	return cmd
}

func (a *App) webhookMapPrefsCmd() *cobra.Command {
	var clusterID, namespace, configName, file string
	cmd := &cobra.Command{
		Use:   "prefs",
		Short: "Manage the image map used by the MutatingAdmissionPolicy",
	}
	push := &cobra.Command{
		Use:   "push",
		Short: "Write the flattened preferences into the image-map ConfigMap",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			path := firstNonEmptyCLI(file, a.cfg.Preferences)
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read preferences: %w", err)
			}
			prefs, err := catalog.Decode(data, path)
			if err != nil {
				return err
			}
			mappings := webhook.FlattenPreferences(prefs)
			if err := webhook.UpsertImageMap(cmd.Context(), client, namespace, configName, mappings); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "wrote %d digest mappings to ConfigMap/%s in %s (from %s)\n",
				len(mappings), firstNonEmptyCLI(configName, webhook.DefaultImageMapName),
				firstNonEmptyCLI(namespace, webhook.DefaultNamespace), path)
			return nil
		},
	}
	push.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	push.Flags().StringVar(&namespace, "namespace", "", "namespace of the image-map ConfigMap")
	push.Flags().StringVar(&configName, "config-name", "", "image-map ConfigMap name")
	push.Flags().StringVar(&file, "file", "", "preferences file (default from config)")
	cmd.AddCommand(push)
	return cmd
}

func (a *App) webhookMapStatusCmd() *cobra.Command {
	var clusterID, namespace, policyName, configName string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the MutatingAdmissionPolicy installation",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, client, err := a.webhookClient(clusterID)
			if err != nil {
				return err
			}
			dyn, err := a.dynamicClient(cl)
			if err != nil {
				return err
			}
			version := "-"
			if info, verr := client.Discovery().ServerVersion(); verr == nil {
				version = info.GitVersion
			}
			if !kubernetesVersionAtLeast(version, 1, 36) {
				fmt.Fprintf(a.Out, "MutatingAdmissionPolicy requires Kubernetes 1.36+ (this cluster is %s)\n", version)
				return nil
			}
			opts := webhook.PolicyOptions{Namespace: namespace, PolicyName: policyName, ConfigName: configName}
			status, err := webhook.StatusPolicy(cmd.Context(), client, dyn, opts)
			if err != nil {
				return err
			}
			a.writeTable(
				[]string{"CLUSTER", "K8S", "POLICY", "BINDING", "CONFIGMAP", "MAPPINGS"},
				[][]string{{
					cl.ID, version, present(status.Policy, status.PolicyName),
					present(status.Binding, status.BindingName),
					present(status.ConfigMap, status.ConfigName),
					fmt.Sprintf("%d", status.Mappings),
				}},
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&clusterID, "cluster", "", "cluster id (default: first configured)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace of the image-map ConfigMap")
	cmd.Flags().StringVar(&policyName, "policy-name", "", "MutatingAdmissionPolicy/binding name")
	cmd.Flags().StringVar(&configName, "config-name", "", "image-map ConfigMap name")
	return cmd
}

func (a *App) dynamicClient(cl config.Cluster) (dynamic.Interface, error) {
	restCfg, err := kube.NewRESTConfig(cl.Context, a.kubeconfigPath(cl))
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(restCfg)
}

func present(ok bool, name string) string {
	if !ok {
		return "-"
	}
	return name
}
