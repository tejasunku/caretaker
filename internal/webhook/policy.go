package webhook

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/tejasunku/caretaker/internal/catalog"
)

// MutatingAdmissionPolicy resources used by the MAP mode. They are created
// through the dynamic client so caretaker does not need to track the API
// version that introduced them.
const (
	DefaultPolicyName   = "caretaker-images"
	DefaultImageMapName = "caretaker-image-map"
	PolicyAPIVersion    = "admissionregistration.k8s.io/v1"
)

var (
	policyGVR  = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingadmissionpolicies"}
	bindingGVR = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingadmissionpolicybindings"}
)

// PolicyOptions describes a MutatingAdmissionPolicy installation.
type PolicyOptions struct {
	Namespace     string
	PolicyName    string
	BindingName   string
	ConfigName    string
	Preferences   []byte
	FailurePolicy string
	Namespaces    []string
}

func (o *PolicyOptions) defaults() {
	o.Namespace = firstNonEmpty(o.Namespace, DefaultNamespace)
	o.PolicyName = firstNonEmpty(o.PolicyName, DefaultPolicyName)
	o.BindingName = firstNonEmpty(o.BindingName, o.PolicyName)
	o.ConfigName = firstNonEmpty(o.ConfigName, DefaultImageMapName)
	o.FailurePolicy = firstNonEmpty(o.FailurePolicy, "Ignore")
}

// InstallPolicy writes the digest-to-target ConfigMap and creates or updates
// the MutatingAdmissionPolicy and its binding. Nothing runs in the cluster: the
// control plane evaluates the policy with CEL.
func InstallPolicy(ctx context.Context, client kubernetes.Interface, dyn dynamic.Interface, opts PolicyOptions) error {
	opts.defaults()
	if len(opts.Preferences) == 0 {
		return fmt.Errorf("preferences document is required")
	}
	prefs, err := catalog.Decode(opts.Preferences, "preferences")
	if err != nil {
		return err
	}
	if err := ensureNamespace(ctx, client, opts.Namespace); err != nil {
		return err
	}
	if err := UpsertImageMap(ctx, client, opts.Namespace, opts.ConfigName, FlattenPreferences(prefs)); err != nil {
		return err
	}
	if err := upsertUnstructured(ctx, dyn, policyGVR, mapPolicy(opts)); err != nil {
		return err
	}
	return upsertUnstructured(ctx, dyn, bindingGVR, mapBinding(opts))
}

// UninstallPolicy removes the binding, policy, and image-map ConfigMap. The
// namespace is left in place.
func UninstallPolicy(ctx context.Context, client kubernetes.Interface, dyn dynamic.Interface, opts PolicyOptions) error {
	opts.defaults()
	if err := dyn.Resource(bindingGVR).Delete(ctx, opts.BindingName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := dyn.Resource(policyGVR).Delete(ctx, opts.PolicyName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := client.CoreV1().ConfigMaps(opts.Namespace).Delete(ctx, opts.ConfigName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// PolicyStatus reports whether the policy and binding exist and how many
// digests the image map carries.
type PolicyStatus struct {
	Policy      bool
	Binding     bool
	ConfigMap   bool
	Mappings    int
	PolicyName  string
	BindingName string
	ConfigName  string
}

// StatusPolicy inspects the cluster for a MAP installation.
func StatusPolicy(ctx context.Context, client kubernetes.Interface, dyn dynamic.Interface, opts PolicyOptions) (PolicyStatus, error) {
	opts.defaults()
	status := PolicyStatus{
		PolicyName:  opts.PolicyName,
		BindingName: opts.BindingName,
		ConfigName:  opts.ConfigName,
	}
	if _, err := dyn.Resource(policyGVR).Get(ctx, opts.PolicyName, metav1.GetOptions{}); err == nil {
		status.Policy = true
	} else if !apierrors.IsNotFound(err) {
		return status, err
	}
	if _, err := dyn.Resource(bindingGVR).Get(ctx, opts.BindingName, metav1.GetOptions{}); err == nil {
		status.Binding = true
	} else if !apierrors.IsNotFound(err) {
		return status, err
	}
	cm, err := client.CoreV1().ConfigMaps(opts.Namespace).Get(ctx, opts.ConfigName, metav1.GetOptions{})
	if err == nil {
		status.ConfigMap = true
		status.Mappings = len(cm.Data)
	} else if !apierrors.IsNotFound(err) {
		return status, err
	}
	return status, nil
}

// UpsertImageMap creates or replaces the flattened digest-to-target ConfigMap.
func UpsertImageMap(ctx context.Context, client kubernetes.Interface, namespace, name string, data map[string]string) error {
	namespace = firstNonEmpty(namespace, DefaultNamespace)
	name = firstNonEmpty(name, DefaultImageMapName)
	configMaps := client.CoreV1().ConfigMaps(namespace)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm, err := configMaps.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = configMaps.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Data:       data,
			}, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		cm.Data = data
		_, err = configMaps.Update(ctx, cm, metav1.UpdateOptions{})
		return err
	})
}

// FlattenPreferences turns the preferences document into the ConfigMap the
// policy reads: every base and alias digest maps to its target. Keys are the
// repository with '/' and ':' replaced by '_', followed by the digest hex.
func FlattenPreferences(prefs *catalog.File) map[string]string {
	out := map[string]string{}
	if prefs == nil {
		return out
	}
	for _, service := range prefs.Services {
		for _, mapping := range service.Mappings {
			if mapping.Target == "" {
				continue
			}
			refs := append([]string{mapping.Base}, mapping.Aliases...)
			for _, ref := range refs {
				if key, ok := imageMapKey(ref); ok {
					out[key] = mapping.Target
				}
			}
		}
	}
	return out
}

// imageMapKey mirrors the CEL expression in mapPatchExpression. ConfigMap keys
// cannot contain '/', ':' or '@'.
func imageMapKey(ref string) (string, bool) {
	repo, digest, ok := strings.Cut(ref, "@")
	if !ok {
		return "", false
	}
	_, hex, ok := strings.Cut(digest, ":")
	if !ok || hex == "" {
		return "", false
	}
	repo = strings.ReplaceAll(repo, "/", "_")
	repo = strings.ReplaceAll(repo, ":", "_")
	return repo + "_" + hex, true
}

func mapPolicy(opts PolicyOptions) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": PolicyAPIVersion,
		"kind":       "MutatingAdmissionPolicy",
		"metadata": map[string]interface{}{
			"name": opts.PolicyName,
		},
		"spec": map[string]interface{}{
			"failurePolicy":      opts.FailurePolicy,
			"reinvocationPolicy": "IfNeeded",
			"matchConstraints": map[string]interface{}{
				"resourceRules": []interface{}{map[string]interface{}{
					"apiGroups":   []interface{}{"apps"},
					"apiVersions": []interface{}{"v1"},
					"operations":  []interface{}{"CREATE", "UPDATE"},
					"resources":   []interface{}{"deployments"},
				}},
			},
			"paramKind": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			"mutations": []interface{}{map[string]interface{}{
				"patchType": "JSONPatch",
				"jsonPatch": map[string]interface{}{
					"expression": mapPatchExpression(),
				},
			}},
		},
	}}
	return obj
}

func mapBinding(opts PolicyOptions) *unstructured.Unstructured {
	selector := map[string]interface{}{}
	if len(opts.Namespaces) > 0 {
		values := make([]interface{}, 0, len(opts.Namespaces))
		for _, ns := range opts.Namespaces {
			values = append(values, ns)
		}
		selector = map[string]interface{}{
			"matchExpressions": []interface{}{map[string]interface{}{
				"key":      "kubernetes.io/metadata.name",
				"operator": "In",
				"values":   values,
			}},
		}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": PolicyAPIVersion,
		"kind":       "MutatingAdmissionPolicyBinding",
		"metadata": map[string]interface{}{
			"name": opts.BindingName,
		},
		"spec": map[string]interface{}{
			"policyName": opts.PolicyName,
			"paramRef": map[string]interface{}{
				"name":                    opts.ConfigName,
				"namespace":               opts.Namespace,
				"parameterNotFoundAction": "Allow",
			},
			"matchResources": map[string]interface{}{
				"namespaceSelector": selector,
			},
		},
	}}
}

// mapPatchExpression builds the CEL JSONPatch expression. Mapped digests are
// replaced with their target; everything else is emitted as a no-op test so
// unmapped images pass through untouched. Both regular and init containers are
// covered.
func mapPatchExpression() string {
	return mapListExpression("containers") + "\n+ (has(object.spec.template.spec.initContainers) ? " +
		mapListExpression("initContainers") + " : [])"
}

func mapListExpression(list string) string {
	key := fmt.Sprintf(
		"object.spec.template.spec.%[1]s[i].image.split('@')[0].replace('/', '_').replace(':', '_') + \"_\" + object.spec.template.spec.%[1]s[i].image.split('@')[1].split(':')[1]",
		list,
	)
	return fmt.Sprintf(`lists.range(size(object.spec.template.spec.%[1]s)).map(i,
  object.spec.template.spec.%[1]s[i].image.contains('@') && (%[2]s) in params.data
  ? JSONPatch{op: "replace", path: "/spec/template/spec/%[1]s/" + string(i) + "/image", value: params.data[%[2]s]}
  : JSONPatch{op: "test", path: "/spec/template/spec/%[1]s/" + string(i) + "/image", value: object.spec.template.spec.%[1]s[i].image})`, list, key)
}

func upsertUnstructured(ctx context.Context, dyn dynamic.Interface, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
	name := obj.GetName()
	current, err := dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = dyn.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	obj.SetResourceVersion(current.GetResourceVersion())
	_, err = dyn.Resource(gvr).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// SortedKeys is a small helper for deterministic output and tests.
func SortedKeys(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
