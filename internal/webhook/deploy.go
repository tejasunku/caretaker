package webhook

import (
	"context"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// Defaults for the in-cluster deployment.
const (
	DefaultNamespace    = "caretaker-system"
	DefaultServiceName  = "caretaker-webhook"
	DefaultConfigName   = "caretaker-preferences"
	DefaultPort         = int32(8443)
	DefaultServicePort  = int32(443)
	preferencesMountDir = "/etc/caretaker"
	certsMountDir       = "/certs"
)

// DeployOptions describes the in-cluster webhook installation.
type DeployOptions struct {
	Namespace         string
	ServiceName       string
	Image             string
	Preferences       []byte
	CertDir           string
	RegistryURL       string
	RegistryNamespace string
	RegistryInsecure  bool
	FailurePolicy     string
	Namespaces        []string
}

// Deploy installs the webhook as an in-cluster Deployment and Service, creates
// the preferences ConfigMap and TLS Secret, and registers the
// MutatingWebhookConfiguration against the Service. Nothing on the workstation
// is contacted at admission time.
func Deploy(ctx context.Context, client kubernetes.Interface, opts DeployOptions) error {
	if opts.Image == "" {
		return fmt.Errorf("webhook image is required")
	}
	opts.Namespace = firstNonEmpty(opts.Namespace, DefaultNamespace)
	opts.ServiceName = firstNonEmpty(opts.ServiceName, DefaultServiceName)
	opts.CertDir = firstNonEmpty(opts.CertDir, ".caretaker/webhook")
	if len(opts.Preferences) == 0 {
		return fmt.Errorf("preferences document is required")
	}

	hosts := ServiceHosts(opts.ServiceName, opts.Namespace)
	certs, err := EnsureCertsFor(opts.CertDir, hosts, false)
	if err != nil {
		return err
	}

	if err := ensureNamespace(ctx, client, opts.Namespace); err != nil {
		return err
	}
	if err := UpsertPreferences(ctx, client, opts.Namespace, DefaultConfigName, opts.Preferences); err != nil {
		return err
	}
	if err := upsertSecret(ctx, client, opts.Namespace, opts.ServiceName, certs); err != nil {
		return err
	}
	if err := upsertDeployment(ctx, client, opts); err != nil {
		return err
	}
	if err := upsertService(ctx, client, opts); err != nil {
		return err
	}
	return Install(ctx, client, InstallOptions{
		Service: &ServiceReference{
			Name:      opts.ServiceName,
			Namespace: opts.Namespace,
			Path:      "/mutate",
			Port:      DefaultServicePort,
		},
		CABundle:       certs.CACert,
		Namespaces:     opts.Namespaces,
		FailurePolicy:  opts.FailurePolicy,
		TimeoutSeconds: 5,
	})
}

// Undeploy removes the Deployment, Service, Secret, ConfigMap, and webhook
// registration. The namespace is left in place.
func Undeploy(ctx context.Context, client kubernetes.Interface, namespace, serviceName string) error {
	namespace = firstNonEmpty(namespace, DefaultNamespace)
	serviceName = firstNonEmpty(serviceName, DefaultServiceName)
	if err := Uninstall(ctx, client); err != nil {
		return err
	}
	deletes := []func() error{
		func() error {
			return ignoreNotFound(client.AppsV1().Deployments(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{}))
		},
		func() error {
			return ignoreNotFound(client.CoreV1().Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{}))
		},
		func() error {
			return ignoreNotFound(client.CoreV1().Secrets(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{}))
		},
		func() error {
			return ignoreNotFound(client.CoreV1().ConfigMaps(namespace).Delete(ctx, DefaultConfigName, metav1.DeleteOptions{}))
		},
	}
	for _, del := range deletes {
		if err := del(); err != nil {
			return err
		}
	}
	return nil
}

// UpsertPreferences writes the preferences document to a ConfigMap. This is the
// optional live update path: the webhook rereads the mounted file, and the
// kubelet refreshes it within about a minute.
func UpsertPreferences(ctx context.Context, client kubernetes.Interface, namespace, name string, data []byte) error {
	namespace = firstNonEmpty(namespace, DefaultNamespace)
	name = firstNonEmpty(name, DefaultConfigName)
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "caretaker"},
		},
		Data: map[string]string{"preferences.yaml": string(data)},
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = client.CoreV1().ConfigMaps(namespace).Create(ctx, desired, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		desired.ResourceVersion = existing.ResourceVersion
		_, err = client.CoreV1().ConfigMaps(namespace).Update(ctx, desired, metav1.UpdateOptions{})
		return err
	})
}

// ServiceHosts returns the DNS names the API server uses for an in-cluster
// webhook Service.
func ServiceHosts(name, namespace string) []string {
	return []string{
		fmt.Sprintf("%s.%s.svc", name, namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace),
	}
}

func ensureNamespace(ctx context.Context, client kubernetes.Interface, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "caretaker"},
		},
	}, metav1.CreateOptions{})
	return err
}

func upsertSecret(ctx context.Context, client kubernetes.Interface, namespace, name string, certs *Certs) error {
	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certs.ServerCert,
			corev1.TLSPrivateKeyKey: certs.ServerKey,
			"ca.crt":                certs.CACert,
		},
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = client.CoreV1().Secrets(namespace).Create(ctx, desired, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		desired.ResourceVersion = existing.ResourceVersion
		_, err = client.CoreV1().Secrets(namespace).Update(ctx, desired, metav1.UpdateOptions{})
		return err
	})
}

func upsertDeployment(ctx context.Context, client kubernetes.Interface, opts DeployOptions) error {
	labels := map[string]string{"app": opts.ServiceName}
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.ServiceName,
			Namespace: opts.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true)},
					Containers: []corev1.Container{{
						Name:            "webhook",
						Image:           opts.Image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/usr/local/bin/caretaker"},
						Args: []string{
							"webhook", "serve",
							"--addr", fmt.Sprintf(":%d", DefaultPort),
							"--cert-dir", certsMountDir,
							"--preferences", preferencesMountDir + "/preferences.yaml",
						},
						Env: []corev1.EnvVar{
							{Name: "CARETAKER_REGISTRY_URL", Value: opts.RegistryURL},
							{Name: "CARETAKER_REGISTRY_NAMESPACE", Value: opts.RegistryNamespace},
							{Name: "CARETAKER_REGISTRY_INSECURE", Value: fmt.Sprintf("%t", opts.RegistryInsecure)},
						},
						Ports: []corev1.ContainerPort{{Name: "https", ContainerPort: DefaultPort}},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "certs", MountPath: certsMountDir, ReadOnly: true},
							{Name: "preferences", MountPath: preferencesMountDir, ReadOnly: true},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
								Path:   "/healthz",
								Port:   intstr.FromString("https"),
								Scheme: corev1.URISchemeHTTPS,
							}},
							InitialDelaySeconds: 2,
							PeriodSeconds:       10,
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: boolPtr(false),
							ReadOnlyRootFilesystem:   boolPtr(true),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "certs", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: opts.ServiceName}}},
						{Name: "preferences", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: DefaultConfigName},
						}}},
					},
				},
			},
		},
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := client.AppsV1().Deployments(opts.Namespace).Get(ctx, opts.ServiceName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = client.AppsV1().Deployments(opts.Namespace).Create(ctx, desired, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		desired.ResourceVersion = existing.ResourceVersion
		_, err = client.AppsV1().Deployments(opts.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
		return err
	})
}

func upsertService(ctx context.Context, client kubernetes.Interface, opts DeployOptions) error {
	labels := map[string]string{"app": opts.ServiceName}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.ServiceName,
			Namespace: opts.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "https",
				Port:       DefaultServicePort,
				TargetPort: intstr.FromString("https"),
			}},
		},
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := client.CoreV1().Services(opts.Namespace).Get(ctx, opts.ServiceName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = client.CoreV1().Services(opts.Namespace).Create(ctx, desired, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		desired.ResourceVersion = existing.ResourceVersion
		desired.Spec.ClusterIP = existing.Spec.ClusterIP
		_, err = client.CoreV1().Services(opts.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
		return err
	})
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func boolPtr(v bool) *bool    { return &v }
func int32Ptr(v int32) *int32 { return &v }

// ReadFile is a small helper so callers can pass a path directly.
func ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
