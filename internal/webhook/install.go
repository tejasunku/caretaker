package webhook

import (
	"context"
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ConfigurationName is the MutatingWebhookConfiguration caretaker manages.
const ConfigurationName = "caretaker-deployments"

// ServiceReference points the API server at an in-cluster webhook Service.
type ServiceReference struct {
	Name      string
	Namespace string
	Path      string
	Port      int32
}

// InstallOptions describes the webhook registration. The webhook always runs
// in-cluster, so a Service reference is required.
type InstallOptions struct {
	Service        *ServiceReference
	CABundle       []byte
	Namespaces     []string
	FailurePolicy  string
	TimeoutSeconds int32
}

// Install creates or updates the MutatingWebhookConfiguration.
func Install(ctx context.Context, client kubernetes.Interface, opts InstallOptions) error {
	if opts.Service == nil || opts.Service.Name == "" {
		return fmt.Errorf("webhook service reference is required")
	}
	if opts.Service.Namespace == "" {
		return fmt.Errorf("webhook service namespace is required")
	}
	if len(opts.CABundle) == 0 {
		return fmt.Errorf("webhook CA bundle is required")
	}
	timeout := opts.TimeoutSeconds
	if timeout == 0 {
		timeout = 5
	}
	failure := admissionregistrationv1.FailurePolicyType(opts.FailurePolicy)
	switch failure {
	case "":
		failure = admissionregistrationv1.Ignore
	case admissionregistrationv1.Ignore, admissionregistrationv1.Fail:
	default:
		return fmt.Errorf("invalid failure policy %q (want Ignore or Fail)", opts.FailurePolicy)
	}
	sideEffects := admissionregistrationv1.SideEffectClassNone
	reinvocation := admissionregistrationv1.IfNeededReinvocationPolicy
	matchPolicy := admissionregistrationv1.Equivalent
	scope := admissionregistrationv1.NamespacedScope

	clientCfg := clientConfig(opts.Service)
	clientCfg.CABundle = opts.CABundle
	hook := admissionregistrationv1.MutatingWebhook{
		Name:                    "deployments.caretaker.io",
		AdmissionReviewVersions: []string{"v1"},
		SideEffects:             &sideEffects,
		FailurePolicy:           &failure,
		MatchPolicy:             &matchPolicy,
		ReinvocationPolicy:      &reinvocation,
		TimeoutSeconds:          &timeout,
		ClientConfig:            clientCfg,
		Rules: []admissionregistrationv1.RuleWithOperations{{
			Operations: []admissionregistrationv1.OperationType{
				admissionregistrationv1.Create,
				admissionregistrationv1.Update,
			},
			Rule: admissionregistrationv1.Rule{
				APIGroups:   []string{"apps"},
				APIVersions: []string{"v1"},
				Resources:   []string{"deployments"},
				Scope:       &scope,
			},
		}},
	}
	if len(opts.Namespaces) > 0 {
		hook.NamespaceSelector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "kubernetes.io/metadata.name",
				Operator: metav1.LabelSelectorOpIn,
				Values:   opts.Namespaces,
			}},
		}
	}

	desired := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: ConfigurationName},
		Webhooks:   []admissionregistrationv1.MutatingWebhook{hook},
	}

	existing, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, ConfigurationName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	_, err = client.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func clientConfig(service *ServiceReference) admissionregistrationv1.WebhookClientConfig {
	port := service.Port
	if port == 0 {
		port = 443
	}
	path := service.Path
	if path == "" {
		path = "/mutate"
	}
	return admissionregistrationv1.WebhookClientConfig{
		Service: &admissionregistrationv1.ServiceReference{
			Name:      service.Name,
			Namespace: service.Namespace,
			Path:      &path,
			Port:      &port,
		},
	}
}

// Uninstall removes the webhook configuration.
func Uninstall(ctx context.Context, client kubernetes.Interface) error {
	err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(ctx, ConfigurationName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// Status returns the installed configuration, or nil when it is absent.
func Status(ctx context.Context, client kubernetes.Interface) (*admissionregistrationv1.MutatingWebhookConfiguration, error) {
	config, err := client.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, ConfigurationName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}
