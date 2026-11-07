package webhook

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testPreferences = "apiVersion: caretaker/v1alpha2\nkind: PatchPreferences\nservices: {}\n"

func TestDeployCreatesInClusterResources(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := DeployOptions{
		Namespace:   "caretaker-system",
		ServiceName: "caretaker-webhook",
		Image:       "localhost:5000/test/caretaker-webhook@sha256:abc",
		Preferences: []byte(testPreferences),
		CertDir:     t.TempDir(),
	}

	if err := Deploy(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Namespaces().Get(ctx, "caretaker-system", metav1.GetOptions{}); err != nil {
		t.Fatalf("namespace: %v", err)
	}
	cm, err := client.CoreV1().ConfigMaps("caretaker-system").Get(ctx, DefaultConfigName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap: %v", err)
	}
	if cm.Data["preferences.yaml"] != testPreferences {
		t.Errorf("configmap data = %q", cm.Data["preferences.yaml"])
	}
	secret, err := client.CoreV1().Secrets("caretaker-system").Get(ctx, "caretaker-webhook", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret: %v", err)
	}
	if len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 {
		t.Error("secret is missing TLS material")
	}
	dep, err := client.AppsV1().Deployments("caretaker-system").Get(ctx, "caretaker-webhook", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != opts.Image {
		t.Errorf("image = %s", dep.Spec.Template.Spec.Containers[0].Image)
	}
	svc, err := client.CoreV1().Services("caretaker-system").Get(ctx, "caretaker-webhook", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	if svc.Spec.Ports[0].Port != DefaultServicePort {
		t.Errorf("service port = %d", svc.Spec.Ports[0].Port)
	}
	hook := installedHook(t, ctx, client)
	if hook.ClientConfig.Service == nil {
		t.Fatal("webhook should use the in-cluster Service")
	}
	if hook.ClientConfig.Service.Name != "caretaker-webhook" {
		t.Errorf("service name = %s", hook.ClientConfig.Service.Name)
	}
}

func TestUndeployRemovesEverything(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	opts := DeployOptions{
		Namespace:   "caretaker-system",
		ServiceName: "caretaker-webhook",
		Image:       "localhost:5000/test/caretaker-webhook@sha256:abc",
		Preferences: []byte(testPreferences),
		CertDir:     t.TempDir(),
	}
	if err := Deploy(ctx, client, opts); err != nil {
		t.Fatal(err)
	}
	if err := Undeploy(ctx, client, "caretaker-system", "caretaker-webhook"); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Status(ctx, client); err != nil || cfg != nil {
		t.Fatalf("webhook config still present: %v %v", cfg, err)
	}
	if _, err := client.AppsV1().Deployments("caretaker-system").Get(ctx, "caretaker-webhook", metav1.GetOptions{}); err == nil {
		t.Error("deployment still present")
	}
	if _, err := client.CoreV1().ConfigMaps("caretaker-system").Get(ctx, DefaultConfigName, metav1.GetOptions{}); err == nil {
		t.Error("configmap still present")
	}
}
