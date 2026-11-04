// Package prefs distributes the preference map through the OCI registry.
//
// Each push stores the whole preferences document as one JSON blob inside an
// OCI artifact anchored to a tagged index manifest, mirroring the lineage
// design. The blob contains image digests only as strings — no image is ever a
// subject — so the document survives even after every image it mentions has
// been deleted from the registry. The artifact's own digest is the version
// identity: push returns it and pull takes it.
//
// Lineage and preferences are deliberately independent for now; cross-checking
// them is the doctor command's job later.
package prefs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tejasunku/caretaker/internal/catalog"
)

// Artifact types caretaker publishes for the preference map.
const (
	// MediaTypePreferences is one stored version of the whole document.
	MediaTypePreferences = "application/vnd.caretaker.preferences.v1+json"
	// MediaTypeIndex anchors the preference history in the registry.
	MediaTypeIndex = "application/vnd.caretaker.preferences.index.v1+json"
)

// IndexTag is the tag of the immutable preference index manifest. It is the
// subject anchor for every stored version, so it stays tagged (and therefore
// live) even after the images named in the document are deleted.
const IndexTag = "caretaker-preferences-index"

// RepositoryName is the default repository, under the registry namespace, that
// holds the preference history.
const RepositoryName = "caretaker-preferences"

// Annotations recorded on each stored version, so a later listing can order
// versions and attribute them without downloading every blob.
const (
	AnnotationKind   = "caretaker.io/preferences"
	AnnotationSetAt  = "caretaker.io/set-at"
	AnnotationWriter = "caretaker.io/writer"

	KindIndex   = "index"
	KindVersion = "version"
)

// Store is the registry surface prefs needs. *registry.Client implements it.
type Store interface {
	Exists(ctx context.Context, ref string) (bool, error)
	Resolve(ctx context.Context, ref string) (string, error)
	EnsureArtifactTag(ctx context.Context, repository, tag, artifactType string, annotations map[string]string) (string, bool, error)
	PushBlobArtifact(ctx context.Context, subjectRef, artifactType string, payload []byte, annotations map[string]string) (string, error)
	GetBlobArtifact(ctx context.Context, ref string) ([]byte, error)
}

// Client stores and retrieves preference documents in the registry.
type Client struct {
	Registry   Store
	Repository string
	Writer     string
	Now        func() time.Time
}

// Push stores file as a new immutable version and returns its digest.
func (c *Client) Push(ctx context.Context, file *catalog.File) (string, error) {
	repository, err := c.repository()
	if err != nil {
		return "", err
	}
	if file == nil {
		return "", errors.New("no preferences to push")
	}
	if err := file.Validate(); err != nil {
		return "", fmt.Errorf("refusing to push invalid preferences: %w", err)
	}
	payload, err := json.Marshal(file)
	if err != nil {
		return "", fmt.Errorf("encode preferences: %w", err)
	}
	indexDigest, _, err := c.Registry.EnsureArtifactTag(ctx, repository, IndexTag, MediaTypeIndex, map[string]string{
		AnnotationKind: KindIndex,
	})
	if err != nil {
		return "", fmt.Errorf("preferences index for %s: %w", repository, err)
	}
	annotations := map[string]string{
		AnnotationKind:  KindVersion,
		AnnotationSetAt: c.now().UTC().Format(time.RFC3339Nano),
	}
	if c.Writer != "" {
		annotations[AnnotationWriter] = c.Writer
	}
	digest, err := c.Registry.PushBlobArtifact(ctx, repository+"@"+indexDigest, MediaTypePreferences, payload, annotations)
	if err != nil {
		return "", fmt.Errorf("push preferences to %s: %w", repository, err)
	}
	return digest, nil
}

// Pull decodes the preferences version stored at digest. digest may be a bare
// "sha256:..." or a full "repo@sha256:..." reference. It returns the decoded
// document and the fully qualified reference.
func (c *Client) Pull(ctx context.Context, digest string) (*catalog.File, string, error) {
	ref, err := c.ref(digest)
	if err != nil {
		return nil, "", err
	}
	payload, err := c.Registry.GetBlobArtifact(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	file, err := catalog.Decode(payload, ref)
	if err != nil {
		return nil, "", err
	}
	return file, ref, nil
}

func (c *Client) ref(raw string) (string, error) {
	repository, err := c.repository()
	if err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("a preferences digest is required")
	}
	if strings.HasPrefix(raw, "sha256:") && !strings.Contains(raw, "/") {
		return repository + "@" + raw, nil
	}
	if strings.Contains(raw, "@") {
		repo, digest, ok := strings.Cut(raw, "@")
		if !ok || repo == "" || !strings.HasPrefix(digest, "sha256:") {
			return "", fmt.Errorf("invalid preferences reference %q (want repo@sha256:...)", raw)
		}
		return repo + "@" + digest, nil
	}
	return "", fmt.Errorf("preferences %q is not a digest; stored versions are addressed by sha256 digest", raw)
}

func (c *Client) repository() (string, error) {
	repo := strings.TrimSuffix(strings.TrimSpace(c.Repository), "/")
	if repo == "" {
		return "", errors.New("preferences repository is not configured")
	}
	return repo, nil
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}
