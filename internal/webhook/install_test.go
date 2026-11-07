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
		Service:        &ServiceReference{Name: "caretaker-webhook", Namespace: "caretaker-system"},
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
	opts.Service.Port = 8443
	if err := Install(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	hook = installedHook(t, ctx, client)
	if hook.ClientConfig.Service == nil || hook.ClientConfig.Service.Port == nil || *hook.ClientConfig.Service.Port != 8443 {
		t.Errorf("service = %+v", hook.ClientConfig.Service)
	}
}

func TestInstallCanRestrictNamespaces(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := InstallOptions{
		Service:    &ServiceReference{Name: "caretaker-webhook", Namespace: "caretaker-system"},
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
	opts := InstallOptions{Service: &ServiceReference{Name: "caretaker-webhook", Namespace: "caretaker-system"}, CABundle: []byte("ca")}

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

func TestInstallWithServiceReference(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := InstallOptions{
		Service: &ServiceReference{
			Name:      "caretaker-webhook",
			Namespace: "caretaker-system",
			Path:      "/mutate",
			Port:      443,
		},
		CABundle: []byte("ca"),
	}
	if err := Install(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	hook := installedHook(t, ctx, client)
	if hook.ClientConfig.URL != nil {
		t.Fatalf("expected service mode, got URL %v", *hook.ClientConfig.URL)
	}
	svc := hook.ClientConfig.Service
	if svc == nil {
		t.Fatal("service reference is nil")
	}
	if svc.Name != "caretaker-webhook" || svc.Namespace != "caretaker-system" {
		t.Errorf("service = %s/%s", svc.Namespace, svc.Name)
	}
	if svc.Port == nil || *svc.Port != 443 {
		t.Errorf("port = %v", svc.Port)
	}
	if svc.Path == nil || *svc.Path != "/mutate" {
		t.Errorf("path = %v", svc.Path)
	}
}

func TestInstallRequiresEndpoint(t *testing.T) {
	if err := Install(context.Background(), fake.NewSimpleClientset(), InstallOptions{CABundle: []byte("ca")}); err == nil {
		t.Fatal("expected an error without a service reference")
	}
}
