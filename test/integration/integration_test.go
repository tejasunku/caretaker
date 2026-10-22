//go:build integration

// Package integration exercises caretaker against a real registry and kind
// cluster. It is opt-in:
//
//	CARETAKER_TEST_CONTEXT=kind-caretaker go test -tags integration ./test/integration/...
//
// The patch test additionally requires BuildKit and Copa:
//
//	CARETAKER_TEST_CONTEXT=kind-caretaker CARETAKER_TEST_PATCH=1 go test -tags integration ./test/integration/...
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/config"
	"github.com/tejasunku/caretaker/internal/health"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/patch"
	"github.com/tejasunku/caretaker/internal/registry"
	"github.com/tejasunku/caretaker/internal/runner"
	"github.com/tejasunku/caretaker/internal/update"
)

const rolloutTimeout = 2 * time.Minute

type fixture struct {
	cl          *kube.Client
	reg         *registry.Client
	prefs       *catalog.File
	deploy      kube.Deployment
	repo        string
	targetTag   string
	targetImage string
}

func TestUpdateThenRollback(t *testing.T) {
	f := newFixture(t)
	defer restore(t, f)

	u := updater(f.reg)
	results := u.UpdateCluster(testContext(t), f.cl, []string{f.deploy.Service}, f.prefs, update.Options{
		Verify: true, AutoRollback: true, Timeout: rolloutTimeout,
	})
	if len(results) != 1 || results[0].Err != nil || results[0].Action != "updated" {
		t.Fatalf("update results = %+v", results)
	}

	after := getDeployment(t, f.cl, f.deploy.Service)
	if after.Image != f.targetImage {
		t.Errorf("image = %q, want %q", after.Image, f.targetImage)
	}
	if got := after.Annotations[kube.AnnotationLKG]; got != f.targetImage {
		t.Errorf("LKG = %q, want %q", got, f.targetImage)
	}
	if got := after.Annotations[kube.AnnotationPrevious]; got != f.deploy.Image {
		t.Errorf("previous = %q, want %q", got, f.deploy.Image)
	}

	res := u.Rollback(testContext(t), f.cl, f.deploy.Service, "", update.Options{Timeout: rolloutTimeout})
	if res.Err != nil || res.Action != "rolled-back" {
		t.Fatalf("rollback = %+v", res)
	}
	rolledBack := getDeployment(t, f.cl, f.deploy.Service)
	if rolledBack.Image != f.deploy.Image {
		t.Errorf("after rollback image = %q, want %q", rolledBack.Image, f.deploy.Image)
	}
}

func TestFailedHealthCheckRollsBack(t *testing.T) {
	f := newFixture(t)
	defer restore(t, f)

	u := updater(f.reg)
	results := u.UpdateCluster(testContext(t), f.cl, []string{f.deploy.Service}, f.prefs, update.Options{
		Verify: true, AutoRollback: true, Timeout: rolloutTimeout,
		HealthCommand: "exit 1",
	})
	if len(results) != 1 || results[0].Err == nil || results[0].Action != "rolled-back" {
		t.Fatalf("results = %+v", results)
	}

	after := getDeployment(t, f.cl, f.deploy.Service)
	if after.Image != f.deploy.Image {
		t.Errorf("image = %q, want rollback to %q", after.Image, f.deploy.Image)
	}
	if got := after.Annotations[kube.AnnotationLKG]; got != f.deploy.Image {
		t.Errorf("LKG = %q, want %q", got, f.deploy.Image)
	}
}

func TestPatchService(t *testing.T) {
	if os.Getenv("CARETAKER_TEST_PATCH") == "" {
		t.Skip("set CARETAKER_TEST_PATCH=1 to run the Copa patch round trip")
	}
	ctx := testContext(t)
	reg := testRegistry()
	service := env("CARETAKER_TEST_PATCH_SERVICE", "auth")
	repo := repoFor(service)

	p := &patch.Patcher{
		Runner:       runner.Exec{Stdout: os.Stdout, Stderr: os.Stderr},
		Copa:         env("CARETAKER_TEST_COPA", "../../bin/copa"),
		Trivy:        env("CARETAKER_TEST_TRIVY", "../../bin/trivy"),
		BuildkitAddr: env("CARETAKER_TEST_BUILDKIT", "tcp://127.0.0.1:12345"),
		Platform:     env("CARETAKER_TEST_PLATFORM", "linux/amd64"),
		TagSuffix:    "patched",
		Now:          time.Now,
	}
	res, err := p.PatchService(ctx, reg, service, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped {
		t.Skipf("target already exists: %s", res.Target)
	}
	if res.TargetDigest == "" {
		t.Fatal("patch produced no target digest")
	}
	exists, err := reg.Exists(ctx, res.Target)
	if err != nil || !exists {
		t.Fatalf("target %s not found (exists=%v err=%v)", res.Target, exists, err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := testContext(t)
	contextName := os.Getenv("CARETAKER_TEST_CONTEXT")
	if contextName == "" {
		t.Skip("set CARETAKER_TEST_CONTEXT (e.g. kind-caretaker) to run integration tests")
	}
	service := env("CARETAKER_TEST_SERVICE", "api")
	namespace := env("CARETAKER_TEST_NAMESPACE", "caretaker")

	cl, err := kube.NewClient("integration", contextName, namespace, os.Getenv("CARETAKER_TEST_KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	reg := testRegistry()
	deploy, err := cl.GetDeployment(ctx, service)
	if err != nil {
		t.Fatal(err)
	}

	base, ok := image.BaseVersion(deploy.Ref.Tag)
	if !ok {
		t.Skipf("running tag %q has no numeric base version", deploy.Ref.Tag)
	}
	tags, err := reg.Tags(ctx, repoFor(service))
	if err != nil {
		t.Fatal(err)
	}
	targetTag := alternatePatchedTag(t, tags, deploy.Ref.Tag)
	targetDigest, err := reg.Resolve(ctx, repoFor(service)+":"+targetTag)
	if err != nil {
		t.Fatal(err)
	}

	prefs := catalog.New()
	if err := prefs.Set(service, repoFor(service)+":"+base, repoFor(service)+":"+targetTag, ""); err != nil {
		t.Fatal(err)
	}

	return &fixture{
		cl:          cl,
		reg:         reg,
		prefs:       prefs,
		deploy:      deploy,
		repo:        repoFor(service),
		targetTag:   targetTag,
		targetImage: repoFor(service) + ":" + targetTag + "@" + targetDigest,
	}
}

// alternatePatchedTag finds a patched build of the same base version that is
// different from the tag currently running.
func alternatePatchedTag(t *testing.T, tags []string, currentTag string) string {
	t.Helper()
	base, _ := image.BaseVersion(currentTag)
	for _, tag := range tags {
		if tag == currentTag || !image.IsPatched(tag) {
			continue
		}
		if b, ok := image.BaseVersion(tag); ok && b == base {
			return tag
		}
	}
	t.Skipf("need two patched tags for base %s, have %v", base, tags)
	return ""
}

func updater(reg *registry.Client) *update.Updater {
	return &update.Updater{
		Registry: reg,
		Health:   health.Checker{Runner: runner.Exec{Stdout: os.Stdout, Stderr: os.Stderr}},
		Now:      time.Now,
	}
}

func restore(t *testing.T, f *fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), rolloutTimeout)
	defer cancel()
	if err := f.cl.SetImage(ctx, f.deploy.Service, f.deploy.Image); err != nil {
		t.Logf("restore %s: %v", f.deploy.Service, err)
		return
	}
	if err := f.cl.WaitForRollout(ctx, f.deploy.Service, rolloutTimeout); err != nil {
		t.Logf("restore rollout %s: %v", f.deploy.Service, err)
	}
}

func getDeployment(t *testing.T, cl *kube.Client, service string) kube.Deployment {
	t.Helper()
	d, err := cl.GetDeployment(testContext(t), service)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func testRegistry() *registry.Client {
	return registry.New(config.Registry{
		URL:       env("CARETAKER_TEST_REGISTRY", "localhost:5000"),
		Namespace: env("CARETAKER_TEST_REGISTRY_NAMESPACE", "test"),
		Insecure:  true,
	})
}

func repoFor(service string) string {
	return env("CARETAKER_TEST_REGISTRY", "localhost:5000") + "/" +
		env("CARETAKER_TEST_REGISTRY_NAMESPACE", "test") + "/" + service
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
