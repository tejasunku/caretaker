// Package registry wraps the OCI distribution API caretaker needs: resolving
// digests, listing tags, and enumerating repositories.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"

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

// Artifact is one OCI referrer descriptor.
type Artifact struct {
	Digest       string
	MediaType    string
	ArtifactType string
	Annotations  map[string]string
}

// Referrers lists artifacts attached to a subject reference. It uses the OCI
// Referrers API and falls back to the tag scheme for older registries.
func (c *Client) Referrers(ctx context.Context, subjectRef string) ([]Artifact, error) {
	return c.ReferrersOfType(ctx, subjectRef, "")
}

// ReferrersOfType lists artifacts attached to a subject reference and keeps
// only those whose artifactType matches. An empty artifactType keeps
// everything. The OCI Referrers API exposes artifactType as the one
// server-side filter; annotations are never queryable.
func (c *Client) ReferrersOfType(ctx context.Context, subjectRef, artifactType string) ([]Artifact, error) {
	ref, err := c.parse(subjectRef)
	if err != nil {
		return nil, err
	}
	digestRef, err := c.digestRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	opts := c.options(ctx)
	if artifactType != "" {
		opts = append(opts, remote.WithFilter("artifactType", artifactType))
	}
	idx, err := remote.Referrers(digestRef, opts...)
	if err != nil {
		return nil, fmt.Errorf("referrers for %s: %w", subjectRef, err)
	}
	mf, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("referrers manifest for %s: %w", subjectRef, err)
	}
	out := make([]Artifact, 0, len(mf.Manifests))
	for _, d := range mf.Manifests {
		if artifactType != "" && d.ArtifactType != artifactType {
			continue
		}
		out = append(out, Artifact{
			Digest:       d.Digest.String(),
			MediaType:    string(d.MediaType),
			ArtifactType: d.ArtifactType,
			Annotations:  d.Annotations,
		})
	}
	return out, nil
}

// PushArtifact attaches an OCI artifact to subjectRef and returns its digest.
// The artifact is pushed by digest, so it needs no tag of its own. artifactType
// becomes the manifest's artifactType (via the config media type, the fallback
// OCI defines), which is what referrers queries filter on.
func (c *Client) PushArtifact(ctx context.Context, subjectRef, artifactType string, annotations map[string]string) (string, error) {
	subject, err := c.parse(subjectRef)
	if err != nil {
		return "", err
	}
	desc, err := remote.Head(subject, c.options(ctx)...)
	if err != nil {
		return "", fmt.Errorf("resolve subject %s: %w", subjectRef, err)
	}
	repo := subject.Context()
	img, err := artifactImage(artifactType, annotations, desc)
	if err != nil {
		return "", err
	}
	h, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("artifact digest: %w", err)
	}
	if err := remote.Write(repo.Digest(h.String()), img, c.options(ctx)...); err != nil {
		return "", fmt.Errorf("push artifact for %s: %w", subjectRef, err)
	}
	return h.String(), nil
}

// EnsureArtifactTag returns the digest of the artifact tagged tag in
// repository, creating it when absent. This is how a repository anchors a
// durable, tagged subject (the lineage index) whose digest never changes.
func (c *Client) EnsureArtifactTag(ctx context.Context, repository, tag, artifactType string, annotations map[string]string) (string, bool, error) {
	repo, err := c.parseRepository(repository)
	if err != nil {
		return "", false, err
	}
	ref := repository + ":" + tag
	if exists, err := c.Exists(ctx, ref); err != nil {
		return "", false, err
	} else if exists {
		digest, err := c.Resolve(ctx, ref)
		return digest, false, err
	}
	img, err := artifactImage(artifactType, annotations, nil)
	if err != nil {
		return "", false, err
	}
	h, err := img.Digest()
	if err != nil {
		return "", false, fmt.Errorf("artifact digest: %w", err)
	}
	if err := remote.Write(repo.Tag(tag), img, c.options(ctx)...); err != nil {
		return "", false, fmt.Errorf("push artifact %s: %w", ref, err)
	}
	return h.String(), true, nil
}

// artifactImage builds a subject-less or subject-anchored empty artifact whose
// artifactType is carried by the config media type.
func artifactImage(artifactType string, annotations map[string]string, subject *v1.Descriptor) (v1.Image, error) {
	mt := artifactType
	if mt == "" {
		mt = "application/vnd.oci.empty.v1+json"
	}
	var img v1.Image = empty.Image
	// The subject field only exists on an OCI manifest, so artifacts are always
	// OCI schema, never Docker schema2.
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, types.MediaType(mt))
	if len(annotations) > 0 {
		img = mutate.Annotations(img, annotations).(v1.Image)
	}
	if subject != nil {
		img = mutate.Subject(img, *subject).(v1.Image)
	}
	return img, nil
}

// ArtifactAnnotations reads the manifest annotations of an artifact.
func (c *Client) ArtifactAnnotations(ctx context.Context, ref string) (map[string]string, error) {
	parsed, err := c.parse(ref)
	if err != nil {
		return nil, err
	}
	desc, err := remote.Get(parsed, c.options(ctx)...)
	if err != nil {
		return nil, fmt.Errorf("get artifact %s: %w", ref, err)
	}
	var mf struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(desc.Manifest, &mf); err != nil {
		return nil, fmt.Errorf("parse artifact %s: %w", ref, err)
	}
	return mf.Annotations, nil
}

func (c *Client) digestRef(ctx context.Context, ref name.Reference) (name.Digest, error) {
	if d, ok := ref.(name.Digest); ok {
		return d, nil
	}
	desc, err := remote.Head(ref, c.options(ctx)...)
	if err != nil {
		return name.Digest{}, fmt.Errorf("resolve %s: %w", ref, err)
	}
	return ref.Context().Digest(desc.Digest.String()), nil
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
