//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/tejasunku/caretaker/internal/config"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/registry"
)

// repoBin finds a binary in the repository's bin/ directory no matter which
// directory the test runs from, falling back to PATH. Empty means not found.
func repoBin(t *testing.T, name string) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "bin", name),
		filepath.Join("bin", name),
	}
	if p, err := exec.LookPath(name); err == nil {
		candidates = append(candidates, p)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err != nil {
				return c
			}
			return abs
		}
	}
	return ""
}

const (
	protoHost      = "localhost:5000"
	protoNamespace = "test"
	protoRepo      = protoHost + "/" + protoNamespace + "/proto"
)

func protoRegistry() *registry.Client {
	return registry.New(config.Registry{URL: protoHost, Namespace: protoNamespace, Insecure: true})
}

// pushProtoImage pushes a distinct, minimal image manifest and returns its
// digest-only reference.
func pushProtoImage(t *testing.T, tag string, unique string) string {
	t.Helper()
	ref, err := name.ParseReference(protoRepo+":"+tag, name.Insecure)
	if err != nil {
		t.Fatalf("parse %s: %v", protoRepo+":"+tag, err)
	}
	var img v1.Image = empty.Image
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, types.OCIConfigJSON)
	img = mutate.Annotations(img, map[string]string{
		"caretaker.io/proto":               unique,
		"org.opencontainers.image.created": time.Now().UTC().Format(time.RFC3339),
	}).(v1.Image)
	if err := remote.Write(ref, img, remote.WithAuth(authn.Anonymous)); err != nil {
		t.Fatalf("push %s: %v", ref, err)
	}
	digest, err := img.Digest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return protoRepo + "@" + digest.String()
}

func deleteManifest(t *testing.T, repository, digest string) {
	t.Helper()
	path := strings.TrimPrefix(repository, protoHost+"/")
	u := fmt.Sprintf("http://%s/v2/%s/manifests/%s", protoHost, path, digest)
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete %s: %v", u, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		t.Fatalf("delete %s: status %d: %s", u, resp.StatusCode, body)
	}
}

// referrersAPI issues a raw GET against the Referrers API so the test checks
// the wire contract, not just the client library.
func referrersAPI(t *testing.T, repository, digest, artifactType string) []struct {
	Digest       string            `json:"digest"`
	ArtifactType string            `json:"artifactType"`
	Annotations  map[string]string `json:"annotations"`
} {
	t.Helper()
	path := strings.TrimPrefix(repository, protoHost+"/")
	u := fmt.Sprintf("http://%s/v2/%s/referrers/%s", protoHost, path, digest)
	if artifactType != "" {
		u += "?artifactType=" + url.QueryEscape(artifactType)
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		t.Fatalf("referrers request: %v", err)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", u, resp.StatusCode, body)
	}
	var idx struct {
		Manifests []struct {
			Digest       string            `json:"digest"`
			ArtifactType string            `json:"artifactType"`
			Annotations  map[string]string `json:"annotations"`
			Subject      *struct {
				Digest string `json:"digest"`
			} `json:"subject"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		t.Fatalf("parse referrers index: %v", err)
	}
	out := make([]struct {
		Digest       string            `json:"digest"`
		ArtifactType string            `json:"artifactType"`
		Annotations  map[string]string `json:"annotations"`
	}, 0, len(idx.Manifests))
	for _, m := range idx.Manifests {
		out = append(out, struct {
			Digest       string            `json:"digest"`
			ArtifactType string            `json:"artifactType"`
			Annotations  map[string]string `json:"annotations"`
		}{Digest: m.Digest, ArtifactType: m.ArtifactType, Annotations: m.Annotations})
	}
	return out
}

// TestLineageLogSurvivesBaseDeletion is the registry prototype: build an
// append-only lineage log anchored on a tagged index manifest, then delete the
// base image and confirm the log still answers "what was patched from X?".
func TestLineageLogSurvivesBaseDeletion(t *testing.T) {
	ctx := context.Background()
	reg := protoRegistry()
	rec := &lineage.RecorderClient{Registry: reg}

	baseRef := pushProtoImage(t, "base", "base-"+t.Name())
	childRef := pushProtoImage(t, "child", "child-"+t.Name())

	baseDigest := strings.TrimPrefix(baseRef, protoRepo+"@")
	childDigest := strings.TrimPrefix(childRef, protoRepo+"@")

	if err := rec.Record(ctx, childRef, lineage.Metadata{
		Service:      "proto",
		BaseDigest:   baseDigest,
		BaseVersion:  "1.0",
		SourceDigest: baseDigest,
		ParentDigest: baseDigest,
		PatchedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	// 1. The index manifest exists, is tagged, and is stable.
	indexDigest, _, err := rec.Index(ctx, protoRepo)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	again, _, err := rec.Index(ctx, protoRepo)
	if err != nil || again != indexDigest {
		t.Fatalf("index not stable: %q then %q (err %v)", indexDigest, again, err)
	}
	if exists, err := reg.Exists(ctx, protoRepo+":"+lineage.IndexTag); err != nil || !exists {
		t.Fatalf("index tag missing: exists=%v err=%v", exists, err)
	}

	// 2. The log entry is a referrer of the index with a filterable artifactType.
	raw := referrersAPI(t, protoRepo, indexDigest, lineage.MediaTypeEntry)
	if len(raw) == 0 {
		t.Fatal("raw referrers API returned no lineage entries")
	}
	for _, e := range raw {
		if e.ArtifactType != lineage.MediaTypeEntry {
			t.Errorf("entry %s artifactType = %q, want %q", e.Digest, e.ArtifactType, lineage.MediaTypeEntry)
		}
		if e.Annotations[lineage.AnnotationChildDigest] == "" {
			t.Errorf("entry %s has no child digest annotation", e.Digest)
		}
	}

	filtered, err := reg.ReferrersOfType(ctx, protoRepo+"@"+indexDigest, lineage.MediaTypeEntry)
	if err != nil {
		t.Fatalf("referrers filter: %v", err)
	}
	if len(filtered) == 0 {
		t.Fatal("client-side referrers filter returned nothing")
	}

	// 3. The child still carries its fast-path metadata referrer.
	meta, ok, err := rec.Metadata(ctx, childRef)
	if err != nil || !ok {
		t.Fatalf("child metadata: ok=%v err=%v", ok, err)
	}
	if meta.BaseDigest != baseDigest {
		t.Errorf("child base = %q, want %q", meta.BaseDigest, baseDigest)
	}

	// 4. Delete the base image. Everything keyed to it dies with it.
	deleteManifest(t, protoRepo, baseDigest)
	if exists, err := reg.Exists(ctx, protoRepo+"@"+baseDigest); err == nil && exists {
		t.Fatal("base digest still resolves after delete")
	}

	// 5. Parent-keyed lookups are what a registry may drop for us. Zot only
	// drops them when retention policies set deleteReferrers, which this demo
	// registry does not, so record whichever behaviour we see. The lineage log
	// must not care either way.
	children, err := rec.Children(ctx, protoRepo+"@"+baseDigest)
	if err != nil {
		t.Logf("Children on deleted base errored: %v", err)
	} else {
		t.Logf("Children on deleted base returned %d (zot keeps referrers unless retention deleteReferrers is on)", len(children))
	}

	entries, err := rec.Log(ctx, protoRepo)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("lineage log is empty after base deletion")
	}
	byBase, err := rec.ChildrenForBase(ctx, protoRepo, baseDigest)
	if err != nil {
		t.Fatalf("ChildrenForBase: %v", err)
	}
	if len(byBase) == 0 {
		t.Fatal("ChildrenForBase found nothing after base deletion")
	}
	found := false
	for _, c := range byBase {
		if c.Digest == childDigest {
			found = true
		}
	}
	if !found {
		t.Errorf("ChildrenForBase = %+v, want to include %s", byBase, childDigest)
	}

	// 6. The index itself is still tagged, so the log has a live anchor.
	if exists, err := reg.Exists(ctx, protoRepo+":"+lineage.IndexTag); err != nil || !exists {
		t.Fatalf("index tag lost after base deletion: exists=%v err=%v", exists, err)
	}
}

// TestLineageEntrySignature signs one log entry with cosign (when installed)
// and confirms the signature is itself a referrer of the entry.
func TestLineageEntrySignature(t *testing.T) {
	cosign := repoBin(t, "cosign")
	if cosign == "" {
		t.Skip("cosign not installed; run ./scripts/setup.sh")
	}
	tmp := t.TempDir()
	keyPrefix := filepath.Join(tmp, "caretaker")
	// A signing config with no transparency log keeps the prototype offline:
	// cosign v3 refuses --tlog-upload=false alongside its default config.
	signingConfig := filepath.Join(tmp, "signing-config.json")
	if err := os.WriteFile(signingConfig, []byte(`{"mediaType":"application/vnd.dev.sigstore.signingconfig.v0.2+json","rekorTlogConfig":{},"tsaConfig":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	gen := exec.Command(cosign, "generate-key-pair", "--output-key-prefix", keyPrefix)
	gen.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("cosign generate-key-pair: %v\n%s", err, out)
	}

	ctx := context.Background()
	reg := protoRegistry()
	rec := &lineage.RecorderClient{Registry: reg}

	baseRef := pushProtoImage(t, "signbase", "signbase")
	baseDigest := strings.TrimPrefix(baseRef, protoRepo+"@")
	childRef := pushProtoImage(t, "signchild", "signchild")
	if err := rec.Record(ctx, childRef, lineage.Metadata{
		Service:      "proto",
		BaseDigest:   baseDigest,
		BaseVersion:  "1.0",
		SourceDigest: baseDigest,
		ParentDigest: baseDigest,
		PatchedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	entries, err := rec.Log(ctx, protoRepo)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	var entry lineage.Entry
	for _, e := range entries {
		if e.BaseDigest == baseDigest {
			entry = e
		}
	}
	if entry.Artifact == "" {
		t.Fatalf("no log entry for base %s", baseDigest)
	}

	target := protoRepo + "@" + entry.Artifact
	sign := exec.Command(cosign, "sign", "--yes", "--signing-config", signingConfig, "--key", keyPrefix+".key", target)
	sign.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	if out, err := sign.CombinedOutput(); err != nil {
		t.Fatalf("cosign sign: %v\n%s", err, out)
	}

	verify := exec.Command(cosign, "verify", "--key", keyPrefix+".pub", "--insecure-ignore-tlog=true", "--insecure-ignore-sct=true", target)
	if out, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("cosign verify: %v\n%s", err, out)
	}

	// The signature is a referrer of the lineage entry itself.
	raw := referrersAPI(t, protoRepo, entry.Artifact, "")
	if len(raw) == 0 {
		t.Fatal("signature did not attach as a referrer of the lineage entry")
	}
	t.Logf("lineage entry %s has %d referrer(s), first artifactType=%q", entry.Artifact, len(raw), raw[0].ArtifactType)
}
