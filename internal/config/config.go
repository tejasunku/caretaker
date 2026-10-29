// Package config loads caretaker's YAML configuration and applies environment
// overrides. Precedence is: defaults < file < environment.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk configuration for the CLI.
type Config struct {
	Path        string    `yaml:"-"`
	Registry    Registry  `yaml:"registry"`
	Patch       Patch     `yaml:"patch"`
	Kubeconfig  string    `yaml:"kubeconfig,omitempty"`
	Services    []string  `yaml:"services"`
	Preferences string    `yaml:"preferences"`
	Clusters    []Cluster `yaml:"clusters"`
	Webhook     Webhook   `yaml:"webhook"`
	// AllowTags opts into tag-based input and discovery. Digests are the
	// default identity; tags may be deleted and are not assumed to exist.
	AllowTags bool `yaml:"allowTags,omitempty"`
}

// Webhook configures the optional Deployment mutating admission webhook.
type Webhook struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	CertDir       string `yaml:"certDir"`
	FailurePolicy string `yaml:"failurePolicy"`
}

// Registry describes the OCI registry caretaker reads and pushes to.
type Registry struct {
	URL       string `yaml:"url"`
	Namespace string `yaml:"namespace"`
	Insecure  bool   `yaml:"insecure"`
	Username  string `yaml:"username,omitempty"`
	Password  string `yaml:"password,omitempty"`
}

// Patch describes how Copa and Trivy are invoked.
type Patch struct {
	Cluster      string `yaml:"cluster,omitempty"`
	Copa         string `yaml:"copa"`
	Trivy        string `yaml:"trivy"`
	BuildkitAddr string `yaml:"buildkitAddr"`
	Platform     string `yaml:"platform"`
	TagSuffix    string `yaml:"tagSuffix"`
	Report       string `yaml:"report,omitempty"`
	// Sources maps a service to the digest it should be patched from. It is the
	// digest-first replacement for discovering the newest tag.
	Sources map[string]string `yaml:"sources,omitempty"`
	// ParentPolicy controls what to do when a source digest is gone:
	// prefer-parent (default), oldest-child, newest-child, or none.
	ParentPolicy string `yaml:"parentPolicy,omitempty"`
	// SkipIfClean scans the source with Trivy and skips patching when it finds
	// no vulnerabilities.
	SkipIfClean bool `yaml:"skipIfClean,omitempty"`
	// Concurrency is the number of images patched at once by "patch all".
	Concurrency int `yaml:"concurrency,omitempty"`
}

// Cluster maps a friendly id to a kube-context and namespace.
type Cluster struct {
	ID         string `yaml:"id"`
	Context    string `yaml:"context"`
	Namespace  string `yaml:"namespace"`
	Stage      string `yaml:"stage,omitempty"`
	Kubeconfig string `yaml:"kubeconfig,omitempty"`
}

// Default returns the built-in configuration used when no file is present.
func Default() *Config {
	return &Config{
		Registry: Registry{
			URL:       "localhost:5000",
			Namespace: "test",
			Insecure:  true,
		},
		Patch: Patch{
			Copa:         "./bin/copa",
			Trivy:        "./bin/trivy",
			BuildkitAddr: "tcp://127.0.0.1:12345",
			Platform:     "linux/amd64",
			TagSuffix:    "patched",
			ParentPolicy: "prefer-parent",
			Concurrency:  1,
		},
		Services:    []string{"api", "auth", "cache", "payments", "web", "worker"},
		Preferences: "config/preferences.yaml",
		Webhook: Webhook{
			Host:          "10.89.2.1",
			Port:          8443,
			CertDir:       ".caretaker/webhook",
			FailurePolicy: "Ignore",
		},
	}
}

// Load reads path (or $CARETAKER_CONFIG, or caretaker.yaml) and applies env
// overrides. A missing file is not an error: built-in defaults are used.
func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("CARETAKER_CONFIG")
	}
	if path == "" {
		path = "caretaker.yaml"
	}

	cfg := Default()
	cfg.Path = path

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	case errors.Is(err, fs.ErrNotExist):
		// Defaults only.
	default:
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	cfg.applyEnv()
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks invariants that every command relies on.
func (c *Config) Validate() error {
	if c.Registry.URL == "" {
		return errors.New("registry.url is required")
	}
	if c.Registry.Namespace == "" {
		return errors.New("registry.namespace is required")
	}
	if c.Patch.Platform == "" {
		return errors.New("patch.platform is required")
	}
	if c.Webhook.Port <= 0 || c.Webhook.Port > 65535 {
		return fmt.Errorf("webhook.port %d is invalid", c.Webhook.Port)
	}
	if len(c.Services) == 0 {
		return errors.New("services must not be empty")
	}
	for i, cl := range c.Clusters {
		if cl.ID == "" {
			return fmt.Errorf("clusters[%d].id is required", i)
		}
	}
	return nil
}

// ClusterByID looks up a configured cluster, falling back to treating id as a
// kube-context name with the provided namespace.
func (c *Config) ClusterByID(id, namespace string) (Cluster, bool) {
	for _, cl := range c.Clusters {
		if cl.ID == id {
			return cl, true
		}
	}
	if id == "" {
		return Cluster{}, false
	}
	if namespace == "" {
		namespace = "default"
	}
	return Cluster{ID: id, Context: id, Namespace: namespace}, true
}

// ResolveClusters expands "all"/"" or a comma-separated id list.
func (c *Config) ResolveClusters(selector, namespace string) ([]Cluster, error) {
	if selector == "" || selector == "all" {
		if len(c.Clusters) == 0 {
			return nil, errors.New("no clusters configured")
		}
		return c.Clusters, nil
	}
	var out []Cluster
	for _, id := range splitList(selector) {
		cl, ok := c.ClusterByID(id, namespace)
		if !ok {
			return nil, fmt.Errorf("unknown cluster %q", id)
		}
		out = append(out, cl)
	}
	return out, nil
}

// ResolveServices expands "all"/"" or a comma-separated service list.
func (c *Config) ResolveServices(selector string) ([]string, error) {
	if selector == "" || selector == "all" {
		return append([]string(nil), c.Services...), nil
	}
	wanted := splitList(selector)
	known := make(map[string]bool, len(c.Services))
	for _, s := range c.Services {
		known[s] = true
	}
	for _, s := range wanted {
		if !known[s] {
			return nil, fmt.Errorf("unknown service %q", s)
		}
	}
	return wanted, nil
}

// Repository returns the full repository for a service image.
func (c *Config) Repository(service string) string {
	return strings.TrimSuffix(c.Registry.URL, "/") + "/" + strings.Trim(c.Registry.Namespace, "/") + "/" + service
}

func (c *Config) normalize() {
	c.Registry.URL = strings.TrimSuffix(c.Registry.URL, "/")
	c.Registry.Namespace = strings.Trim(c.Registry.Namespace, "/")
	c.Patch.TagSuffix = strings.TrimPrefix(c.Patch.TagSuffix, "-")
	if c.Patch.Concurrency < 1 {
		c.Patch.Concurrency = 1
	}
	for i := range c.Clusters {
		if c.Clusters[i].Context == "" {
			c.Clusters[i].Context = c.Clusters[i].ID
		}
		if c.Clusters[i].Namespace == "" {
			c.Clusters[i].Namespace = "default"
		}
	}
}

func (c *Config) applyEnv() {
	envString(&c.Registry.URL, "CARETAKER_REGISTRY_URL")
	envString(&c.Registry.Namespace, "CARETAKER_REGISTRY_NAMESPACE")
	envBool(&c.Registry.Insecure, "CARETAKER_REGISTRY_INSECURE")
	envString(&c.Registry.Username, "CARETAKER_REGISTRY_USERNAME")
	envString(&c.Registry.Password, "CARETAKER_REGISTRY_PASSWORD")

	envString(&c.Patch.Cluster, "CARETAKER_PATCH_CLUSTER")
	envString(&c.Patch.Copa, "CARETAKER_COPA")
	envString(&c.Patch.Trivy, "CARETAKER_TRIVY")
	envString(&c.Patch.BuildkitAddr, "CARETAKER_BUILDKIT_ADDR")
	envString(&c.Patch.Platform, "CARETAKER_PLATFORM")
	envString(&c.Patch.TagSuffix, "CARETAKER_TAG_SUFFIX")
	envString(&c.Patch.Report, "CARETAKER_REPORT")
	envString(&c.Patch.ParentPolicy, "CARETAKER_PARENT_POLICY")
	envBool(&c.Patch.SkipIfClean, "CARETAKER_SKIP_IF_CLEAN")
	envInt(&c.Patch.Concurrency, "CARETAKER_PATCH_CONCURRENCY")
	envBool(&c.AllowTags, "CARETAKER_ALLOW_TAGS")

	envString(&c.Kubeconfig, "CARETAKER_KUBECONFIG")
	envString(&c.Preferences, "CARETAKER_PREFERENCES")
	envString(&c.Webhook.Host, "CARETAKER_WEBHOOK_HOST")
	envInt(&c.Webhook.Port, "CARETAKER_WEBHOOK_PORT")
	envString(&c.Webhook.CertDir, "CARETAKER_WEBHOOK_CERT_DIR")
	envString(&c.Webhook.FailurePolicy, "CARETAKER_WEBHOOK_FAILURE_POLICY")

	if v := os.Getenv("CARETAKER_SERVICES"); v != "" {
		c.Services = splitList(v)
	}
}

func envString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func envInt(dst *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func envBool(dst *bool, key string) {
	v := os.Getenv(key)
	if v == "" {
		return
	}
	if b, err := strconv.ParseBool(v); err == nil {
		*dst = b
	}
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
