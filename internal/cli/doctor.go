package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/update"
	"github.com/tejasunku/caretaker/internal/webhook"
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
				detail := fmt.Sprintf("%s/%s (%d deployments)", cl.Context, cl.Namespace, len(deployments))
				version := ""
				if v, verr := a.clusterVersion(ctx, cl); verr == nil {
					version = v
					detail += ", " + v
				}
				add("cluster/"+cl.ID, "ok", detail)

				// Admission rewriting is invisible from the Deployments alone:
				// show what is installed and warn when its copy of the
				// preferences has drifted from the file on disk.
				_, admissionClient, werr := a.webhookClient(cl.ID)
				if werr != nil {
					add("webhook/"+cl.ID, "error", werr.Error())
				} else {
					wstate, wdetail, hInstalled := doctorWebhook(ctx, admissionClient, prefs, a.cfg.Preferences)
					add("webhook/"+cl.ID, wstate, wdetail)
					if kubernetesVersionAtLeast(version, 1, 36) {
						dyn, derr := a.dynamicClient(cl)
						if derr != nil {
							add("policymap/"+cl.ID, "error", derr.Error())
						} else {
							mstate, mdetail, mInstalled := doctorPolicyMap(ctx, admissionClient, dyn, prefs, a.cfg.Preferences)
							add("policymap/"+cl.ID, mstate, mdetail)
							if hInstalled && mInstalled {
								add("admission/"+cl.ID, "warn", "both the webhook and a MutatingAdmissionPolicy are installed; pick one")
							}
						}
					}
				}

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
		issues = append(issues, "no preferred mapping (set-target falls back to the lineage index)")
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

// doctorWebhook reports the in-cluster webhook installation and whether the
// preferences copy it serves still matches prefsPath. installed is true when a
// MutatingWebhookConfiguration is registered.
func doctorWebhook(ctx context.Context, client kubernetes.Interface, prefs *catalog.File, prefsPath string) (state, detail string, installed bool) {
	cfg, err := webhook.Status(ctx, client)
	if err != nil {
		return "error", err.Error(), false
	}
	if cfg == nil {
		return "ok", "not installed", false
	}

	namespace := webhook.DefaultNamespace
	serviceName := webhook.DefaultServiceName
	endpoint := "-"
	if len(cfg.Webhooks) > 0 {
		hook := cfg.Webhooks[0]
		if hook.ClientConfig.Service != nil {
			svc := hook.ClientConfig.Service
			namespace = svc.Namespace
			serviceName = svc.Name
			port := int32(webhook.DefaultServicePort)
			if svc.Port != nil {
				port = *svc.Port
			}
			path := "/mutate"
			if svc.Path != nil {
				path = *svc.Path
			}
			endpoint = fmt.Sprintf("service://%s.%s:%d%s", svc.Name, svc.Namespace, port, path)
		}
	}

	issues := []string{}
	if _, err := client.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{}); err != nil {
		issues = append(issues, "service/"+serviceName+" missing")
	}
	dep, err := client.AppsV1().Deployments(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		issues = append(issues, "deployment/"+serviceName+" missing")
	} else if dep.Status.ReadyReplicas == 0 {
		issues = append(issues, fmt.Sprintf("deployment/%s not ready (0 ready)", serviceName))
	}
	if prefs != nil {
		if changed, err := changedWebhookServices(ctx, client, namespace, prefs); err != nil {
			issues = append(issues, "cannot read served preferences: "+err.Error())
		} else if len(changed) > 0 {
			issues = append(issues, "preferences stale vs "+prefsPath+" ("+strings.Join(changed, ", ")+"); run webhook prefs push")
		}
	}
	if len(issues) > 0 {
		return "warn", endpoint + "; " + strings.Join(issues, "; "), true
	}
	return "ok", endpoint, true
}

// changedWebhookServices lists services whose served mapping targets differ
// from the local preferences document.
func changedWebhookServices(ctx context.Context, client kubernetes.Interface, namespace string, local *catalog.File) ([]string, error) {
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, webhook.DefaultConfigName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	served, err := catalog.Decode([]byte(cm.Data["preferences.yaml"]), "webhook preferences")
	if err != nil {
		return nil, err
	}
	return changedServices(local, served), nil
}

// doctorPolicyMap reports the MutatingAdmissionPolicy installation and whether
// its flattened image map matches prefsPath.
func doctorPolicyMap(ctx context.Context, client kubernetes.Interface, dyn dynamic.Interface, prefs *catalog.File, prefsPath string) (state, detail string, installed bool) {
	status, err := webhook.StatusPolicy(ctx, client, dyn, webhook.PolicyOptions{})
	if err != nil {
		return "error", err.Error(), false
	}
	installed = status.Policy || status.Binding || status.ConfigMap
	if !installed {
		return "ok", "not installed", false
	}
	if !status.Policy || !status.Binding || !status.ConfigMap {
		return "warn", fmt.Sprintf("incomplete: policy=%t binding=%t configmap=%t", status.Policy, status.Binding, status.ConfigMap), true
	}
	detail = fmt.Sprintf("%s + binding, %d image map entries", status.PolicyName, status.Mappings)
	if prefs != nil {
		cm, err := client.CoreV1().ConfigMaps(webhook.DefaultNamespace).Get(ctx, status.ConfigName, metav1.GetOptions{})
		if err != nil {
			return "warn", detail + "; cannot read image map: " + err.Error(), true
		}
		if !maps.Equal(cm.Data, webhook.FlattenPreferences(prefs)) {
			return "warn", detail + "; image map stale vs " + prefsPath + " (run webhook map prefs push)", true
		}
	}
	return "ok", detail, true
}

// changedServices compares mapping targets keyed by service and base, returning
// the services that differ in either direction.
func changedServices(local, remote *catalog.File) []string {
	targets := func(f *catalog.File) map[string]string {
		out := map[string]string{}
		for name, svc := range f.Services {
			for _, m := range svc.Mappings {
				out[name+"\x00"+m.Base] = m.Target
			}
		}
		return out
	}
	left, right := targets(local), targets(remote)
	changed := map[string]bool{}
	for key, target := range left {
		if right[key] != target {
			changed[strings.SplitN(key, "\x00", 2)[0]] = true
		}
	}
	for key, target := range right {
		if left[key] != target {
			changed[strings.SplitN(key, "\x00", 2)[0]] = true
		}
	}
	out := make([]string, 0, len(changed))
	for name := range changed {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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
