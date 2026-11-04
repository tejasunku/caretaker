//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/prefs"
)

// TestPreferencePushPullSignature stores a preferences document in zot, signs
// the stored version, pulls it back by digest, and finally deletes the index
// anchor to prove the version is still readable — the blob holds digests as
// plain strings, so it does not depend on any image or anchor surviving.
func TestPreferencePushPullSignature(t *testing.T) {
	cosign := repoBin(t, "cosign")
	if cosign == "" {
		t.Skip("cosign not installed; run ./scripts/setup.sh")
	}
	ctx := context.Background()
	reg := protoRegistry()
	repo := protoHost + "/" + protoNamespace + "/prefs-proto"
	client := &prefs.Client{
		Registry:   reg,
		Repository: repo,
		Writer:     "integration",
		Now:        time.Now,
	}

	doc := &catalog.File{
		APIVersion: catalog.APIVersion,
		Kind:       catalog.Kind,
		Services: map[string]catalog.Service{
			"api": {Mappings: []catalog.Mapping{{
				Base:        repo + "@sha256:" + strings.Repeat("a", 64),
				Target:      repo + "@sha256:" + strings.Repeat("b", 64),
				BaseVersion: "1.0",
				PatchedAt:   time.Now().UTC().Format(time.RFC3339),
			}}},
		},
	}

	digest, err := client.Push(ctx, doc)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	t.Logf("stored preferences version %s@%s", repo, digest)

	got, ref, err := client.Pull(ctx, digest)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if ref != repo+"@"+digest {
		t.Fatalf("ref = %s, want %s", ref, repo+"@"+digest)
	}
	if !reflect.DeepEqual(got, doc) {
		t.Fatalf("round-trip mismatch:\n got %#v\nwant %#v", got, doc)
	}

	tmp := t.TempDir()
	keyPrefix := filepath.Join(tmp, "caretaker")
	gen := exec.Command(cosign, "generate-key-pair", "--output-key-prefix", keyPrefix)
	gen.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("cosign generate-key-pair: %v\n%s", err, out)
	}

	if err := prefs.Sign(ctx, cosign, ref, keyPrefix+".key"); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := prefs.Verify(ctx, cosign, ref, keyPrefix+".pub"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	arts, err := reg.ReferrersOfType(ctx, ref, "application/vnd.dev.sigstore.bundle.v0.3+json")
	if err != nil {
		t.Fatalf("signature referrers: %v", err)
	}
	if len(arts) == 0 {
		t.Fatal("signature did not attach as a referrer of the preferences version")
	}

	// Remove the tagged index anchor. The version manifest and its blob are
	// addressed by digest, so the document must still pull.
	indexDigest, err := reg.Resolve(ctx, repo+":"+prefs.IndexTag)
	if err != nil {
		t.Fatalf("resolve index: %v", err)
	}
	deleteManifest(t, repo, indexDigest)

	got, _, err = client.Pull(ctx, digest)
	if err != nil {
		t.Fatalf("pull after index deletion: %v", err)
	}
	if !reflect.DeepEqual(got, doc) {
		t.Fatal("document changed after the index anchor was deleted")
	}
	t.Logf("version %s still readable after deleting %s", digest, prefs.IndexTag)
}
