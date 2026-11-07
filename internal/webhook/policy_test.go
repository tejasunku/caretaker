package webhook

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/tejasunku/caretaker/internal/catalog"
)

const policyPreferences = `apiVersion: caretaker/v1alpha2
kind: PatchPreferences
services:
  api:
    mappings:
      - base: localhost:5000/test/api@sha256:aaaa
        aliases:
          - localhost:5000/test/api@sha256:bbbb
        target: localhost:5000/test/api@sha256:cccc
`

func TestFlattenPreferences(t *testing.T) {
	prefs, err := catalog.Decode([]byte(policyPreferences), "test")
	if err != nil {
		t.Fatal(err)
	}
	got := FlattenPreferences(prefs)
	want := map[string]string{
		"localhost_5000_test_api_aaaa": "localhost:5000/test/api@sha256:cccc",
		"localhost_5000_test_api_bbbb": "localhost:5000/test/api@sha256:cccc",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d: %v", len(got), len(want), got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestMapPatchExpressionCoversContainers(t *testing.T) {
	expr := mapPatchExpression()
	for _, needle := range []string{
		"object.spec.template.spec.containers",
		"object.spec.template.spec.initContainers",
		"params.data",
		`path: "/spec/template/spec/containers/"`,
		`path: "/spec/template/spec/initContainers/"`,
	} {
		if !strings.Contains(expr, needle) {
			t.Errorf("expression is missing %q", needle)
		}
	}
	if strings.Count(expr, "params.data[") != 2 {
		t.Errorf("expected one lookup per container list, got:\n%s", expr)
	}
}

func TestInstallPolicyCreatesResources(t *testing.T) {
	ctx := context.Background()
	client := kubefake.NewSimpleClientset()
	dyn := newFakeDynamic()
	opts := PolicyOptions{
		Namespace:   "caretaker-system",
		Preferences: []byte(policyPreferences),
	}

	if err := InstallPolicy(ctx, client, dyn, opts); err != nil {
		t.Fatal(err)
	}
	cm, err := client.CoreV1().ConfigMaps("caretaker-system").Get(ctx, DefaultImageMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("configmap: %v", err)
	}
	if len(cm.Data) != 2 {
		t.Errorf("configmap mappings = %v", cm.Data)
	}
	policy, err := dyn.Resource(policyGVR).Get(ctx, DefaultPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if got, _, _ := unstructuredString(policy.Object, "spec", "paramKind", "kind"); got != "ConfigMap" {
		t.Errorf("paramKind.kind = %q", got)
	}
	if _, err := dyn.Resource(bindingGVR).Get(ctx, DefaultPolicyName, metav1.GetOptions{}); err != nil {
		t.Fatalf("binding: %v", err)
	}

	status, err := StatusPolicy(ctx, client, dyn, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Policy || !status.Binding || !status.ConfigMap || status.Mappings != 2 {
		t.Errorf("status = %+v", status)
	}

	if err := UninstallPolicy(ctx, client, dyn, opts); err != nil {
		t.Fatal(err)
	}
	status, err = StatusPolicy(ctx, client, dyn, opts)
	if err != nil {
		t.Fatal(err)
	}
	if status.Policy || status.Binding || status.ConfigMap {
		t.Errorf("resources still present after uninstall: %+v", status)
	}
}

func TestInstallPolicyRejectsEmptyPreferences(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	dyn := newFakeDynamic()
	err := InstallPolicy(context.Background(), client, dyn, PolicyOptions{})
	if err == nil || !strings.Contains(err.Error(), "preferences") {
		t.Fatalf("expected preferences error, got %v", err)
	}
}

func TestInstallPolicyNamespaceSelector(t *testing.T) {
	ctx := context.Background()
	client := kubefake.NewSimpleClientset()
	dyn := newFakeDynamic()
	opts := PolicyOptions{
		Namespace:   "caretaker-system",
		Preferences: []byte(policyPreferences),
		Namespaces:  []string{"prod", "staging"},
	}
	if err := InstallPolicy(ctx, client, dyn, opts); err != nil {
		t.Fatal(err)
	}
	binding, err := dyn.Resource(bindingGVR).Get(ctx, DefaultPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	values, _, _ := unstructuredSlice(binding.Object, "spec", "matchResources", "namespaceSelector", "matchExpressions")
	if len(values) != 1 {
		t.Fatalf("matchExpressions = %v", values)
	}
	expr, ok := values[0].(map[string]interface{})
	if !ok {
		t.Fatalf("matchExpression = %T", values[0])
	}
	if expr["operator"] != "In" {
		t.Errorf("operator = %v", expr["operator"])
	}
	if got := expr["values"].([]interface{}); len(got) != 2 || got[0] != "prod" || got[1] != "staging" {
		t.Errorf("values = %v", got)
	}
}

func newFakeDynamic() *fake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		policyGVR:  "MutatingAdmissionPolicyList",
		bindingGVR: "MutatingAdmissionPolicyBindingList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
}

func unstructuredString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	value, found, err := unstructuredNestedField(obj, fields...)
	if err != nil || !found {
		return "", found, err
	}
	s, ok := value.(string)
	return s, ok, nil
}

func unstructuredSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	value, found, err := unstructuredNestedField(obj, fields...)
	if err != nil || !found {
		return nil, found, err
	}
	s, ok := value.([]interface{})
	return s, ok, nil
}

func unstructuredNestedField(obj map[string]interface{}, fields ...string) (interface{}, bool, error) {
	var current interface{} = obj
	for _, field := range fields {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		current, ok = m[field]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}
