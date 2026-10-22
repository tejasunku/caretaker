// Package kube is caretaker's only path to mutating clusters. It exposes a
// narrow Cluster interface so update/rollback logic can be tested with fakes.
package kube

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"

	"github.com/tejasunku/caretaker/internal/image"
)

// Deployment annotations written by caretaker.
const (
	AnnotationManaged     = "caretaker.io/managed"
	AnnotationLKG         = "caretaker.io/last-known-good-image"
	AnnotationLKGAt       = "caretaker.io/last-known-good-at"
	AnnotationPrevious    = "caretaker.io/previous-image"
	AnnotationLastUpdate  = "caretaker.io/last-update-at"
	AnnotationBaseVersion = "caretaker.io/base-version"
)

// Deployment is a service running in a cluster.
type Deployment struct {
	Service     string
	Container   string
	Image       string
	Ref         image.Ref
	Replicas    int32
	Annotations map[string]string
	Generation  int64
}

// Cluster is the subset of Kubernetes caretaker needs. Implementations must be
// safe for sequential use.
type Cluster interface {
	ID() string
	Namespace() string
	ListDeployments(ctx context.Context) ([]Deployment, error)
	GetDeployment(ctx context.Context, service string) (Deployment, error)
	SetImage(ctx context.Context, service, imageRef string) error
	Annotate(ctx context.Context, service string, annotations map[string]string) error
	WaitForRollout(ctx context.Context, service string, timeout time.Duration) error
}

// Client is a client-go backed Cluster.
type Client struct {
	id        string
	namespace string
	clientset kubernetes.Interface
}

var _ Cluster = (*Client)(nil)

// NewClient builds a client for a kube-context. An empty kubeconfig uses the
// standard loading rules ($KUBECONFIG or ~/.kube/config).
func NewClient(id, kubeContext, namespace, kubeconfig string) (*Client, error) {
	if namespace == "" {
		namespace = "default"
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kube context %q: %w", kubeContext, err)
	}
	restCfg.QPS = 50
	restCfg.Burst = 100

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	return &Client{id: id, namespace: namespace, clientset: cs}, nil
}

// NewClientFromConfig lets tests inject a fake clientset.
func NewClientFromConfig(id, namespace string, cs kubernetes.Interface) *Client {
	return &Client{id: id, namespace: namespace, clientset: cs}
}

// NewRESTConfig is exposed for doctor checks.
func NewRESTConfig(kubeContext, kubeconfig string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		overrides.CurrentContext = kubeContext
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

// ID implements Cluster.
func (c *Client) ID() string { return c.id }

// Namespace implements Cluster.
func (c *Client) Namespace() string { return c.namespace }

// ListDeployments implements Cluster.
func (c *Client) ListDeployments(ctx context.Context) ([]Deployment, error) {
	list, err := c.clientset.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments in %s/%s: %w", c.id, c.namespace, err)
	}
	out := make([]Deployment, 0, len(list.Items))
	for i := range list.Items {
		if d, ok := c.toDeployment(&list.Items[i]); ok {
			out = append(out, d)
		}
	}
	return out, nil
}

// GetDeployment implements Cluster.
func (c *Client) GetDeployment(ctx context.Context, service string) (Deployment, error) {
	obj, err := c.clientset.AppsV1().Deployments(c.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return Deployment{}, fmt.Errorf("get deployment %s/%s: %w", c.id, service, err)
	}
	d, ok := c.toDeployment(obj)
	if !ok {
		return Deployment{}, fmt.Errorf("deployment %s/%s has no containers", c.id, service)
	}
	return d, nil
}

// SetImage replaces the image of the deployment's primary container.
func (c *Client) SetImage(ctx context.Context, service, imageRef string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := c.clientset.AppsV1().Deployments(c.namespace).Get(ctx, service, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get deployment %s/%s: %w", c.id, service, err)
		}
		containers := obj.Spec.Template.Spec.Containers
		if len(containers) == 0 {
			return fmt.Errorf("deployment %s/%s has no containers", c.id, service)
		}
		idx := 0
		for i := range containers {
			if containers[i].Name == service {
				idx = i
				break
			}
		}
		if containers[idx].Image == imageRef {
			return nil
		}
		containers[idx].Image = imageRef
		obj.Spec.Template.Spec.Containers = containers
		_, err = c.clientset.AppsV1().Deployments(c.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update deployment %s/%s: %w", c.id, service, err)
		}
		return nil
	})
}

// Annotate merges annotations onto the deployment's metadata.
func (c *Client) Annotate(ctx context.Context, service string, annotations map[string]string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := c.clientset.AppsV1().Deployments(c.namespace).Get(ctx, service, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get deployment %s/%s: %w", c.id, service, err)
		}
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		for k, v := range annotations {
			obj.Annotations[k] = v
		}
		_, err = c.clientset.AppsV1().Deployments(c.namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("annotate deployment %s/%s: %w", c.id, service, err)
		}
		return nil
	})
}

// WaitForRollout blocks until the deployment is fully rolled out.
func (c *Client) WaitForRollout(ctx context.Context, service string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		obj, err := c.clientset.AppsV1().Deployments(c.namespace).Get(ctx, service, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get deployment %s/%s: %w", c.id, service, err)
		}
		if done, reason := rolloutComplete(obj); done {
			return nil
		} else {
			last = reason
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("rollout %s/%s not complete after %s: %s", c.id, service, timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (c *Client) toDeployment(obj *appsv1.Deployment) (Deployment, bool) {
	containers := obj.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return Deployment{}, false
	}
	ctr := containers[0]
	for i := range containers {
		if containers[i].Name == obj.Name {
			ctr = containers[i]
			break
		}
	}
	ref, err := image.Parse(ctr.Image)
	if err != nil {
		// Keep the deployment visible even if the image ref is unusual.
		ref = image.Ref{Repository: ctr.Image}
	}
	replicas := int32(1)
	if obj.Spec.Replicas != nil {
		replicas = *obj.Spec.Replicas
	}
	annotations := make(map[string]string, len(obj.Annotations))
	for k, v := range obj.Annotations {
		annotations[k] = v
	}
	return Deployment{
		Service:     obj.Name,
		Container:   ctr.Name,
		Image:       ctr.Image,
		Ref:         ref,
		Replicas:    replicas,
		Annotations: annotations,
		Generation:  obj.Generation,
	}, true
}

func rolloutComplete(obj *appsv1.Deployment) (bool, string) {
	if obj.Generation > obj.Status.ObservedGeneration {
		return false, "waiting for observedGeneration"
	}
	want := int32(1)
	if obj.Spec.Replicas != nil {
		want = *obj.Spec.Replicas
	}
	if obj.Status.UpdatedReplicas < want {
		return false, fmt.Sprintf("updated %d/%d", obj.Status.UpdatedReplicas, want)
	}
	if obj.Status.Replicas > obj.Status.UpdatedReplicas {
		return false, "old replicas still present"
	}
	if obj.Status.AvailableReplicas < want {
		return false, fmt.Sprintf("available %d/%d", obj.Status.AvailableReplicas, want)
	}
	for _, cond := range obj.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
			return true, ""
		}
	}
	return false, "Available condition not true"
}

// IsNotFound reports whether err came from a missing object.
func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
