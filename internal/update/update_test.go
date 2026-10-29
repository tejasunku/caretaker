package update

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/health"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/runner"
)

const (
	testRepo      = "localhost:5000/test/api"
	baseTag       = testRepo + ":1.0"
	oldTag        = testRepo + ":1.0-patched-20261016"
	newTag        = testRepo + ":1.0-patched-20261022"
	baseDigest    = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	runningDigest = "sha256:" + "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	newDigest     = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	oldImage      = oldTag + "@" + runningDigest
	newImage      = testRepo + "@" + newDigest
	fixedClock    = "2026-10-20T12:00:00Z"
)

func TestUpdateSuccessSetsLKG(t *testing.T) {
	u, cl, prefs := newFixture(t)

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		Verify: true, AutoRollback: true, Timeout: time.Second,
	})

	if len(results) != 1 || results[0].Action != "updated" {
		t.Fatalf("results = %+v", results)
	}
	d := cl.deployments["api"]
	if d.Image != newImage {
		t.Errorf("image = %q, want %q", d.Image, newImage)
	}
	if got := d.Annotations[kube.AnnotationLKG]; got != newImage {
		t.Errorf("LKG = %q, want %q", got, newImage)
	}
	if got := d.Annotations[kube.AnnotationPrevious]; got != oldImage {
		t.Errorf("previous = %q, want %q", got, oldImage)
	}
	if got := d.Annotations[kube.AnnotationLKGAt]; got != fixedClock {
		t.Errorf("LKG timestamp = %q", got)
	}
	if got := d.Annotations[kube.AnnotationBaseVersion]; got != "1.0" {
		t.Errorf("base version = %q, want 1.0", got)
	}
	if got := d.Annotations[kube.AnnotationBaseDigest]; got != baseDigest {
		t.Errorf("base digest = %q, want %q", got, baseDigest)
	}
	if got := d.Annotations[kube.AnnotationSkipWebhook]; got != "false" {
		t.Errorf("skip-webhook = %q, want false", got)
	}
	if len(cl.setCalls) != 1 || cl.setCalls[0] != newImage {
		t.Errorf("set calls = %v", cl.setCalls)
	}
}

func TestUpdateVerificationFailureRollsBack(t *testing.T) {
	u, cl, prefs := newFixture(t)
	cl.failWaits = 1

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		Verify: true, AutoRollback: true, Timeout: time.Second,
	})

	if results[0].Action != "rolled-back" {
		t.Fatalf("action = %q, err = %v", results[0].Action, results[0].Err)
	}
	d := cl.deployments["api"]
	if d.Image != oldImage {
		t.Errorf("image = %q, want rollback to %q", d.Image, oldImage)
	}
	if got := d.Annotations[kube.AnnotationLKG]; got != oldImage {
		t.Errorf("LKG = %q, want %q", got, oldImage)
	}
	if got := d.Annotations[kube.AnnotationSkipWebhook]; got != "true" {
		t.Errorf("skip-webhook = %q, want true after rollback", got)
	}
	wantCalls := []string{newImage, oldImage}
	if strings.Join(cl.setCalls, ",") != strings.Join(wantCalls, ",") {
		t.Errorf("set calls = %v, want %v", cl.setCalls, wantCalls)
	}
}

func TestUpdateWithoutRollbackLeavesFailedImage(t *testing.T) {
	u, cl, prefs := newFixture(t)
	cl.failWaits = 1

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		Verify: true, AutoRollback: false, Timeout: time.Second,
	})

	if results[0].Action != "verify-failed" {
		t.Fatalf("action = %q", results[0].Action)
	}
	if got := cl.deployments["api"].Image; got != newImage {
		t.Errorf("image = %q, want failed image left in place", got)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	u, cl, prefs := newFixture(t)

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		DryRun: true, Verify: true, AutoRollback: true, Timeout: time.Second,
	})

	if results[0].Action != "would-update" {
		t.Fatalf("action = %q", results[0].Action)
	}
	if len(cl.setCalls) != 0 {
		t.Errorf("set calls = %v", cl.setCalls)
	}
	d := cl.deployments["api"]
	for _, key := range []string{kube.AnnotationManaged, kube.AnnotationLKG, kube.AnnotationPrevious, kube.AnnotationLastUpdate} {
		if _, ok := d.Annotations[key]; ok {
			t.Errorf("annotation %q changed on dry run: %v", key, d.Annotations)
		}
	}
}

func TestNoMappingSkips(t *testing.T) {
	u, cl, _ := newFixture(t)

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{
		Verify: true, Timeout: time.Second,
	})

	if results[0].Action != "skipped" || results[0].Reason != "no preferred mapping" {
		t.Fatalf("result = %+v", results[0])
	}
	if len(cl.setCalls) != 0 {
		t.Errorf("set calls = %v", cl.setCalls)
	}
}

func TestExplicitMappingOverridesCatalog(t *testing.T) {
	u, cl, _ := newFixture(t)

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{
		Verify: true, AutoRollback: true, Timeout: time.Second,
		Explicit: map[string]string{"api": newImage},
	})

	if results[0].Action != "updated" {
		t.Fatalf("action = %q, err = %v", results[0].Action, results[0].Err)
	}
	if got := cl.deployments["api"].Image; got != newImage {
		t.Errorf("image = %q", got)
	}
}

func TestRollbackUsesRecordedLKG(t *testing.T) {
	u, cl, _ := newFixture(t)
	d := cl.deployments["api"]
	d.Image = newImage
	d.Ref, _ = image.Parse(newImage)
	d.Annotations[kube.AnnotationLKG] = oldImage
	cl.deployments["api"] = d

	res := u.Rollback(context.Background(), cl, "api", "", Options{Timeout: time.Second})

	if res.Action != "rolled-back" {
		t.Fatalf("action = %q, err = %v", res.Action, res.Err)
	}
	want := testRepo + "@" + runningDigest
	if got := cl.deployments["api"].Image; got != want {
		t.Errorf("image = %q, want %q", got, want)
	}
	if got := cl.deployments["api"].Annotations[kube.AnnotationLKG]; got != want {
		t.Errorf("LKG = %q, want %q", got, want)
	}
	if got := cl.deployments["api"].Annotations[kube.AnnotationSkipWebhook]; got != "true" {
		t.Errorf("skip-webhook = %q, want true after explicit rollback", got)
	}
}

func TestRollbackWithoutTargetSkips(t *testing.T) {
	u, cl, _ := newFixture(t)
	d := cl.deployments["api"]
	delete(d.Annotations, kube.AnnotationLKG)
	cl.deployments["api"] = d

	res := u.Rollback(context.Background(), cl, "api", "", Options{Timeout: time.Second})
	if res.Err != nil || res.Action != "skipped" {
		t.Fatalf("result = %+v", res)
	}
}

func TestUpdateClearsSkipWebhook(t *testing.T) {
	u, cl, prefs := newFixture(t)
	d := cl.deployments["api"]
	d.Annotations[kube.AnnotationSkipWebhook] = "true"
	cl.deployments["api"] = d

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		Verify: true, AutoRollback: true, Timeout: time.Second,
	})
	if results[0].Action != "updated" {
		t.Fatalf("action = %q, err = %v", results[0].Action, results[0].Err)
	}
	if got := cl.deployments["api"].Annotations[kube.AnnotationSkipWebhook]; got != "false" {
		t.Errorf("skip-webhook = %q, want false after a normal update", got)
	}
}

func TestPreferredDoesNotUseNewerBaseVersion(t *testing.T) {
	u, cl, _ := newFixture(t)
	newerBase := "sha256:" + strings.Repeat("d", 64)
	oldTarget := "sha256:" + strings.Repeat("e", 64)
	prefs := catalog.New()
	if err := prefs.Set("api", testRepo+"@"+baseDigest, testRepo+"@"+oldTarget, ""); err != nil {
		t.Fatal(err)
	}
	if err := prefs.Set("api", testRepo+"@"+newerBase, testRepo+"@"+newDigest, ""); err != nil {
		t.Fatal(err)
	}

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		Verify: true, AutoRollback: true, Timeout: time.Second,
	})
	if results[0].Action != "updated" {
		t.Fatalf("action = %q, err = %v", results[0].Action, results[0].Err)
	}
	want := testRepo + "@" + oldTarget
	if got := cl.deployments["api"].Image; got != want {
		t.Errorf("image = %q, want the 1.9 mapping %q (not the 2.0 mapping)", got, want)
	}
}

func newFixture(t *testing.T) (*Updater, *fakeCluster, *catalog.File) {
	t.Helper()
	ref, err := image.Parse(oldImage)
	if err != nil {
		t.Fatal(err)
	}
	cl := &fakeCluster{
		id: "kind-caretaker", ns: "caretaker",
		deployments: map[string]kube.Deployment{
			"api": {
				Service: "api", Container: "api", Image: oldImage, Ref: ref, Replicas: 3,
				Annotations: map[string]string{
					kube.AnnotationBaseDigest:  baseDigest,
					kube.AnnotationBaseVersion: "1.0",
				},
			},
		},
	}
	reg := &fakeRegistry{
		digests: map[string]string{baseTag: baseDigest, newTag: newDigest},
		tags:    map[string][]string{testRepo: {"1.0", "1.0-patched-20261016", "1.0-patched-20261022"}},
	}
	prefs := catalog.New()
	if err := prefs.Set("api", testRepo+"@"+baseDigest, testRepo+"@"+newDigest, ""); err != nil {
		t.Fatal(err)
	}
	u := &Updater{
		Registry: reg,
		Health:   health.Checker{Runner: &runner.Print{}},
		Now:      func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) },
	}
	return u, cl, prefs
}

type fakeCluster struct {
	id          string
	ns          string
	deployments map[string]kube.Deployment
	setCalls    []string
	failWaits   int
}

func (f *fakeCluster) ID() string        { return f.id }
func (f *fakeCluster) Namespace() string { return f.ns }

func (f *fakeCluster) ListDeployments(context.Context) ([]kube.Deployment, error) {
	out := make([]kube.Deployment, 0, len(f.deployments))
	for _, d := range f.deployments {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeCluster) GetDeployment(_ context.Context, service string) (kube.Deployment, error) {
	d, ok := f.deployments[service]
	if !ok {
		return kube.Deployment{}, fmt.Errorf("deployment %s not found", service)
	}
	return d, nil
}

func (f *fakeCluster) SetImage(_ context.Context, service, imageRef string) error {
	d, ok := f.deployments[service]
	if !ok {
		return fmt.Errorf("deployment %s not found", service)
	}
	ref, err := image.Parse(imageRef)
	if err != nil {
		return err
	}
	d.Image = imageRef
	d.Ref = ref
	f.deployments[service] = d
	f.setCalls = append(f.setCalls, imageRef)
	return nil
}

func (f *fakeCluster) Annotate(_ context.Context, service string, annotations map[string]string) error {
	d, ok := f.deployments[service]
	if !ok {
		return fmt.Errorf("deployment %s not found", service)
	}
	if d.Annotations == nil {
		d.Annotations = map[string]string{}
	}
	for k, v := range annotations {
		d.Annotations[k] = v
	}
	f.deployments[service] = d
	return nil
}

func (f *fakeCluster) WaitForRollout(context.Context, string, time.Duration) error {
	if f.failWaits > 0 {
		f.failWaits--
		return errors.New("rollout unhealthy")
	}
	return nil
}

type fakeRegistry struct {
	digests map[string]string
	tags    map[string][]string
}

func (r *fakeRegistry) Resolve(_ context.Context, ref string) (string, error) {
	digest, ok := r.digests[ref]
	if !ok {
		return "", fmt.Errorf("no digest for %s", ref)
	}
	return digest, nil
}

func (r *fakeRegistry) Tags(_ context.Context, repository string) ([]string, error) {
	return r.tags[repository], nil
}

func (r *fakeRegistry) Exists(_ context.Context, ref string) (bool, error) {
	_, ok := r.digests[ref]
	return ok, nil
}

func (r *fakeRegistry) Catalog(context.Context) ([]string, error) { return nil, nil }
