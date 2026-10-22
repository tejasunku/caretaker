// Package registry wraps the OCI distribution API caretaker needs: resolving
// digests, listing tags, and enumerating repositories.
package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/tejasunku/caretaker/internal/config"
	"github.com/tejasunku/caretaker/internal/image"
)

// Registry is the read-only surface caretaker uses. Cyber reasons: narrow
// interfaces are easy to fake in tests.
type Registry interface {
	Resolve(ctx context.Context, ref string) (string, error)
	Tags(ctx context.Context, repository string) ([]string, error)
	Exists(ctx context.Context, ref string) (bool, error)
	Catalog(ctx context.Context) ([]string, error)
}

// Client is an OCI registry client.
type Client struct {
	url       string
	namespace string
	insecure  bool
	auth      authn.Authenticator
}

var _ Registry = (*Client)(nil)

// New builds a client from configuration.
func New(cfg config.Registry) *Client {
	auth := authn.Anonymous
	if cfg.Username != "" || cfg.Password != "" {
		auth = authn.FromConfig(authn.AuthConfig{
			Username: cfg.Username,
			Password: cfg.Password,
		})
	}
	return &Client{
		url:       stripScheme(cfg.URL),
		namespace: strings.Trim(cfg.Namespace, "/"),
		insecure:  cfg.Insecure,
		auth:      auth,
	}
}

// Repository returns the full repository for a service.
func (c *Client) Repository(service string) string {
	return c.url + "/" + c.namespace + "/" + service
}

// FullRepository prefixes a bare catalog path with the registry host.
func (c *Client) FullRepository(path string) string {
	if strings.HasPrefix(path, c.url+"/") {
		return path
	}
	return c.url + "/" + strings.TrimPrefix(path, "/")
}

// Resolve returns the manifest digest for a reference.
func (c *Client) Resolve(ctx context.Context, ref string) (string, error) {
	parsed, err := c.parse(ref)
	if err != nil {
		return "", err
	}
	desc, err := remote.Head(parsed, c.options(ctx)...)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	return desc.Digest.String(), nil
}

// Tags lists the tags for a repository.
func (c *Client) Tags(ctx context.Context, repository string) ([]string, error) {
	repo, err := c.parseRepository(repository)
	if err != nil {
		return nil, err
	}
	tags, err := remote.List(repo, c.options(ctx)...)
	if err != nil {
		return nil, fmt.Errorf("list tags for %s: %w", repository, err)
	}
	return tags, nil
}

// Exists reports whether a reference (usually repo:tag) is present.
func (c *Client) Exists(ctx context.Context, ref string) (bool, error) {
	parsed, err := c.parse(ref)
	if err != nil {
		return false, err
	}
	_, err = remote.Head(parsed, c.options(ctx)...)
	if err == nil {
		return true, nil
	}
	var terr *transport.Error
	if errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("check %s: %w", ref, err)
}

// Catalog lists repository paths (without the registry host).
func (c *Client) Catalog(ctx context.Context) ([]string, error) {
	reg, err := name.NewRegistry(c.url, c.nameOptions()...)
	if err != nil {
		return nil, fmt.Errorf("parse registry %q: %w", c.url, err)
	}
	repos, err := remote.Catalog(ctx, reg, c.options(ctx)...)
	if err != nil {
		return nil, fmt.Errorf("list catalog: %w", err)
	}
	return repos, nil
}

// DigestOf resolves a tag and returns a v1.Hash.
func (c *Client) DigestOf(ctx context.Context, repository, tag string) (v1.Hash, error) {
	ref, err := c.parse(repository + ":" + tag)
	if err != nil {
		return v1.Hash{}, err
	}
	desc, err := remote.Head(ref, c.options(ctx)...)
	if err != nil {
		return v1.Hash{}, err
	}
	return desc.Digest, nil
}

func (c *Client) parse(s string) (name.Reference, error) {
	ref, err := name.ParseReference(s, c.nameOptions()...)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", s, err)
	}
	return ref, nil
}

func (c *Client) parseRepository(s string) (name.Repository, error) {
	repo, err := name.NewRepository(s, c.nameOptions()...)
	if err != nil {
		return name.Repository{}, fmt.Errorf("parse repository %q: %w", s, err)
	}
	return repo, nil
}

func (c *Client) nameOptions() []name.Option {
	if c.insecure {
		return []name.Option{name.Insecure}
	}
	return nil
}

func (c *Client) options(ctx context.Context) []remote.Option {
	return []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuth(c.auth),
	}
}

func stripScheme(s string) string {
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return s
}

// ParseRef is a convenience wrapper used by callers that already hold a ref.
func ParseRef(s string) (image.Ref, error) { return image.Parse(s) }
