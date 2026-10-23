package webhook

import (
	"context"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInstallIsClusterWideByDefault(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := InstallOptions{
		URL:            "https://10.89.2.1:8443/mutate",
		CABundle:       []byte("ca"),
		FailurePolicy:  "Ignore",
		TimeoutSeconds: 5,
	}

	if err := Install(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	hook := installedHook(t, ctx, client)
	if hook.NamespaceSelector != nil {
		t.Fatalf("expected a cluster-wide webhook, got namespaceSelector %+v", hook.NamespaceSelector)
	}
	if hook.Rules[0].Rule.Resources[0] != "deployments" {
		t.Errorf("resources = %v", hook.Rules[0].Rule.Resources)
	}
	if len(hook.Rules[0].Operations) != 2 {
		t.Errorf("operations = %v", hook.Rules[0].Operations)
	}
	if hook.FailurePolicy == nil || *hook.FailurePolicy != admissionregistrationv1.Ignore {
		t.Errorf("failurePolicy = %v", hook.FailurePolicy)
	}

	// Install is create-or-update.
	opts.URL = "https://10.89.2.1:9443/mutate"
	if err := Install(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	hook = installedHook(t, ctx, client)
	if hook.ClientConfig.URL == nil || *hook.ClientConfig.URL != opts.URL {
		t.Errorf("url = %v", hook.ClientConfig.URL)
	}
}

func TestInstallCanRestrictNamespaces(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := InstallOptions{
		URL:        "https://10.89.2.1:8443/mutate",
		CABundle:   []byte("ca"),
		Namespaces: []string{"team-a", "team-b"},
	}

	if err := Install(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	hook := installedHook(t, ctx, client)
	if hook.NamespaceSelector == nil || len(hook.NamespaceSelector.MatchExpressions) != 1 {
		t.Fatalf("namespaceSelector = %+v", hook.NamespaceSelector)
	}
	if len(hook.NamespaceSelector.MatchExpressions[0].Values) != 2 {
		t.Errorf("namespace values = %v", hook.NamespaceSelector.MatchExpressions[0].Values)
	}
}

func TestInstallAfterUninstall(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := InstallOptions{URL: "https://10.89.2.1:8443/mutate", CABundle: []byte("ca")}

	if err := Install(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(ctx, client); err != nil {
		t.Fatal(err)
	}
	config, err := Status(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if config != nil {
		t.Fatal("expected the configuration to be gone")
	}
}

func installedHook(t *testing.T, ctx context.Context, client *fake.Clientset) admissionregistrationv1.MutatingWebhook {
	t.Helper()
	config, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, ConfigurationName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Webhooks) != 1 {
		t.Fatalf("webhooks = %d", len(config.Webhooks))
	}
	return config.Webhooks[0]
}
