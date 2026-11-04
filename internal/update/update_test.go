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
	"github.com/tejasunku/caretaker/internal/lineage"
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
	// LKG is preserved: it is still the last image that passed verification.
	if got := cl.deployments["api"].Annotations[kube.AnnotationLKG]; got != oldImage {
		t.Errorf("LKG = %q, want preserved %q", got, oldImage)
	}
	// The abandoned image becomes previous-image so a rollback can be undone.
	if got := cl.deployments["api"].Annotations[kube.AnnotationPrevious]; got != newImage {
		t.Errorf("previous = %q, want the abandoned image %q", got, newImage)
	}
	if got := cl.deployments["api"].Annotations[kube.AnnotationSkipWebhook]; got != "true" {
		t.Errorf("skip-webhook = %q, want true after explicit rollback", got)
	}

	// A second rollback toggles back to the image we just left.
	res = u.Rollback(context.Background(), cl, "api", "", Options{Timeout: time.Second})
	if res.Action != "rolled-back" {
		t.Fatalf("second action = %q, err = %v", res.Action, res.Err)
	}
	if got := cl.deployments["api"].Image; got != newImage {
		t.Errorf("second rollback image = %q, want toggle back to %q", got, newImage)
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
	digests  map[string]string
	tags     map[string][]string
	tagCalls int
}

func (r *fakeRegistry) Resolve(_ context.Context, ref string) (string, error) {
	digest, ok := r.digests[ref]
	if !ok {
		return "", fmt.Errorf("no digest for %s", ref)
	}
	return digest, nil
}

func (r *fakeRegistry) Tags(_ context.Context, repository string) ([]string, error) {
	r.tagCalls++
	return r.tags[repository], nil
}

func (r *fakeRegistry) Exists(_ context.Context, ref string) (bool, error) {
	_, ok := r.digests[ref]
	return ok, nil
}

func (r *fakeRegistry) Catalog(context.Context) ([]string, error) { return nil, nil }

const extraDigest = "sha256:" + "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

// fakeLineage answers from an in-memory lineage index: metadata attached to a
// child and the recorded children of a base.
type fakeLineage struct {
	metadata map[string]lineage.Metadata
	children map[string][]lineage.Child
}

func (f *fakeLineage) Metadata(_ context.Context, childRef string) (lineage.Metadata, bool, error) {
	meta, ok := f.metadata[childRef]
	return meta, ok, nil
}

func (f *fakeLineage) Children(context.Context, string) ([]lineage.Child, error) { return nil, nil }

func (f *fakeLineage) ChildrenForBase(_ context.Context, repository, baseDigest string) ([]lineage.Child, error) {
	return f.children[repository+"@"+baseDigest], nil
}

// lineageFixture wires a running patched child, an empty preference map, and a
// lineage index that knows two patched children of its base.
func lineageFixture(t *testing.T, runningDigest string) (*Updater, *fakeCluster, *fakeRegistry, *fakeLineage) {
	t.Helper()
	running := testRepo + ":" + "1.0-patched-20261016" + "@" + runningDigest
	ref, err := image.Parse(running)
	if err != nil {
		t.Fatal(err)
	}
	cl := &fakeCluster{
		id: "kind-caretaker", ns: "caretaker",
		deployments: map[string]kube.Deployment{
			"api": {
				Service: "api", Container: "api", Image: running, Ref: ref, Replicas: 1,
				Annotations: map[string]string{kube.AnnotationBaseDigest: baseDigest},
			},
		},
	}
	// Exists/Resolve are keyed by whatever ref string the caller passes in.
	reg := &fakeRegistry{digests: map[string]string{
		testRepo + "@" + runningDigest: runningDigest,
		testRepo + "@" + newDigest:     newDigest,
		testRepo + "@" + extraDigest:   extraDigest,
	}}
	lin := &fakeLineage{
		metadata: map[string]lineage.Metadata{},
		children: map[string][]lineage.Child{
			testRepo + "@" + baseDigest: {
				{Digest: extraDigest, Service: "api", BaseDigest: baseDigest, BaseVersion: "1.0", PatchedAt: "2026-10-01T00:00:00Z"},
				{Digest: newDigest, Service: "api", BaseDigest: baseDigest, BaseVersion: "1.0", PatchedAt: "2026-10-22T00:00:00Z"},
			},
		},
	}
	u := &Updater{
		Registry: reg,
		Lineage:  lin,
		Health:   health.Checker{Runner: &runner.Print{}},
		Now:      func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) },
	}
	return u, cl, reg, lin
}

func TestUpdateFallsBackToLineageIndexWithoutMapping(t *testing.T) {
	u, cl, reg, _ := lineageFixture(t, runningDigest)

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{DryRun: true})

	if results[0].Action != "would-update" {
		t.Fatalf("action = %q, reason = %q, err = %v", results[0].Action, results[0].Reason, results[0].Err)
	}
	if want := testRepo + "@" + newDigest; results[0].To != want {
		t.Errorf("to = %q, want newest child %q", results[0].To, want)
	}
	if reg.tagCalls != 0 {
		t.Errorf("Tags() called %d times; index fallback must never scan tags", reg.tagCalls)
	}
}

func TestUpdatePrefersSurvivingLineageChild(t *testing.T) {
	u, cl, reg, _ := lineageFixture(t, runningDigest)
	// The newest recorded patch was pruned from the registry.
	delete(reg.digests, testRepo+"@"+newDigest)

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{DryRun: true})

	if results[0].Action != "would-update" {
		t.Fatalf("action = %q, reason = %q, err = %v", results[0].Action, results[0].Reason, results[0].Err)
	}
	if want := testRepo + "@" + extraDigest; results[0].To != want {
		t.Errorf("to = %q, want surviving child %q", results[0].To, want)
	}
	if reg.tagCalls != 0 {
		t.Errorf("Tags() called %d times; index fallback must never scan tags", reg.tagCalls)
	}
}

func TestUpdateRecoversBaseFromLineageMetadata(t *testing.T) {
	u, cl, reg, lin := lineageFixture(t, runningDigest)
	// The Deployment has no base-digest annotation, but the running image is a
	// patched child that still carries its own lineage metadata.
	d := cl.deployments["api"]
	delete(d.Annotations, kube.AnnotationBaseDigest)
	cl.deployments["api"] = d
	lin.metadata[testRepo+"@"+runningDigest] = lineage.Metadata{
		Service: "api", BaseDigest: baseDigest, BaseVersion: "1.0",
	}

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{DryRun: true})

	if results[0].Action != "would-update" {
		t.Fatalf("action = %q, reason = %q, err = %v", results[0].Action, results[0].Reason, results[0].Err)
	}
	if want := testRepo + "@" + newDigest; results[0].To != want {
		t.Errorf("to = %q, want %q", results[0].To, want)
	}
	if reg.tagCalls != 0 {
		t.Errorf("Tags() called %d times; metadata lookup must never scan tags", reg.tagCalls)
	}
}

func TestUpdateStillSkipsWithNothingToOffer(t *testing.T) {
	u, cl, _, lin := lineageFixture(t, runningDigest)
	lin.children = map[string][]lineage.Child{}

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{DryRun: true})

	if results[0].Action != "skipped" {
		t.Fatalf("action = %q, want skipped", results[0].Action)
	}
	if results[0].Reason != "no preferred mapping" {
		t.Errorf("reason = %q", results[0].Reason)
	}
}

// The running image is an unrecorded patched image: not a mapped base, not a
// mapped target, no lineage. Recording its own digest as caretaker.io/base-digest
// would poison every later lookup (the annotation short-circuits lineage,
// mapping, and tag resolution), so the annotation must stay unset.
func TestUpdateDoesNotRecordUnprovenBase(t *testing.T) {
	u, cl, prefs := newFixture(t)
	d := cl.deployments["api"]
	delete(d.Annotations, kube.AnnotationBaseDigest)
	delete(d.Annotations, kube.AnnotationBaseVersion)
	cl.deployments["api"] = d

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, prefs, Options{
		Explicit:     map[string]string{"api": newImage},
		Verify:       true,
		AutoRollback: true,
		Timeout:      time.Second,
	})
	if results[0].Action != "updated" {
		t.Fatalf("action = %q, err = %v", results[0].Action, results[0].Err)
	}
	updated := cl.deployments["api"]
	if got := updated.Annotations[kube.AnnotationBaseDigest]; got != "" {
		t.Errorf("base-digest = %q, want unset: the running image is not a proven feature build", got)
	}
	if updated.Annotations[kube.AnnotationManaged] != "true" {
		t.Errorf("managed = %q, want true", updated.Annotations[kube.AnnotationManaged])
	}
	if updated.Image != newImage {
		t.Errorf("image = %q, want %q", updated.Image, newImage)
	}
}

// A real update must still record a truthful base when the running image's
// lineage referrer names it.
func TestUpdateRecordsBaseFromLineageMetadata(t *testing.T) {
	u, cl, _, lin := lineageFixture(t, runningDigest)
	d := cl.deployments["api"]
	delete(d.Annotations, kube.AnnotationBaseDigest)
	cl.deployments["api"] = d
	lin.metadata[testRepo+"@"+runningDigest] = lineage.Metadata{
		Service: "api", BaseDigest: baseDigest, BaseVersion: "1.0",
	}

	results := u.UpdateCluster(context.Background(), cl, []string{"api"}, catalog.New(), Options{
		Verify: true, AutoRollback: true, Timeout: time.Second,
	})
	if results[0].Action != "updated" {
		t.Fatalf("action = %q, reason = %q, err = %v", results[0].Action, results[0].Reason, results[0].Err)
	}
	if got := cl.deployments["api"].Annotations[kube.AnnotationBaseDigest]; got != baseDigest {
		t.Errorf("base-digest = %q, want %q from lineage metadata", got, baseDigest)
	}
}

// --allow-tags must resolve the recorded base version to the feature build
// before the running-digest fallback, which would otherwise shadow it on every
// digest-pinned (or tag-pinned) Deployment.
func TestPreferredAllowTagsResolvesBaseVersion(t *testing.T) {
	u, cl, prefs := newFixture(t)
	u.AllowTags = true
	// The running image is neither a mapped base nor a mapped target; only
	// the recorded base version (1.0) can name the feature build.
	d := cl.deployments["api"]
	delete(d.Annotations, kube.AnnotationBaseDigest)
	cl.deployments["api"] = d

	target, ok := u.Preferred(context.Background(), d, prefs)
	if !ok {
		t.Fatal("Preferred = not found; --allow-tags must resolve base-version before falling back to the running digest")
	}
	if want := newImage; target != want {
		t.Errorf("target = %q, want %q", target, want)
	}
}

func TestAnnotationBaseFromMappedFeatureBuild(t *testing.T) {
	u, _, prefs := newFixture(t)
	ref, err := image.Parse(testRepo + "@" + baseDigest)
	if err != nil {
		t.Fatal(err)
	}
	d := kube.Deployment{
		Service: "api", Container: "api",
		Image: testRepo + "@" + baseDigest, Ref: ref,
		Annotations: map[string]string{},
	}

	got, ok := u.annotationBase(context.Background(), d, prefs)
	if !ok {
		t.Fatal("annotationBase = not found; a mapped feature build is a proven base")
	}
	if got != baseDigest {
		t.Errorf("base-digest = %q, want %q", got, baseDigest)
	}
}

func TestAnnotationBaseFromMappingTarget(t *testing.T) {
	u, _, prefs := newFixture(t)
	ref, err := image.Parse(newImage)
	if err != nil {
		t.Fatal(err)
	}
	d := kube.Deployment{
		Service: "api", Container: "api",
		Image: newImage, Ref: ref,
		Annotations: map[string]string{},
	}

	got, ok := u.annotationBase(context.Background(), d, prefs)
	if !ok {
		t.Fatal("annotationBase = not found; a mapped target recovers its feature build")
	}
	if got != baseDigest {
		t.Errorf("base-digest = %q, want the mapping's base %q", got, baseDigest)
	}
}
