package webhook

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/kube"
)

const (
	testRepo      = "localhost:5000/test/api"
	baseDigest    = "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	runningDigest = "sha256:" + "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	targetDigest  = "sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	oldImage      = testRepo + ":1.0-patched-20261016@" + runningDigest
	targetImage   = testRepo + "@" + targetDigest
)

func TestMutateUsesBaseDigestAnnotation(t *testing.T) {
	d := deployment(oldImage, map[string]string{
		kube.AnnotationBaseDigest:  baseDigest,
		kube.AnnotationBaseVersion: "1.0",
	})
	ops, err := newMutator(t, true).Mutate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].Op != "replace" || ops[0].Path != "/spec/template/spec/containers/0/image" || ops[0].Value != targetImage {
		t.Errorf("image patch = %+v", ops[0])
	}
	annotations, ok := ops[1].Value.(map[string]string)
	if !ok {
		t.Fatalf("annotations patch = %+v", ops[1])
	}
	if ops[1].Op != "replace" || ops[1].Path != "/metadata/annotations" {
		t.Errorf("annotations op = %+v", ops[1])
	}
	if annotations[kube.AnnotationBaseDigest] != baseDigest {
		t.Errorf("base digest = %q", annotations[kube.AnnotationBaseDigest])
	}
	if annotations[kube.AnnotationManaged] != "true" {
		t.Errorf("managed = %q", annotations[kube.AnnotationManaged])
	}
	if annotations[kube.AnnotationWebhook] != "true" {
		t.Errorf("webhook = %q", annotations[kube.AnnotationWebhook])
	}
}

func TestMutateResolvesBaseVersionWithAllowTags(t *testing.T) {
	ops, err := newMutatorWithTags(t, true, true).Mutate(context.Background(), deployment(oldImage, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].Op != "replace" || ops[0].Path != "/spec/template/spec/containers/0/image" || ops[0].Value != targetImage {
		t.Errorf("image patch = %+v", ops[0])
	}
	annotations, ok := ops[1].Value.(map[string]string)
	if !ok {
		t.Fatalf("annotations patch = %+v", ops[1])
	}
	if ops[1].Op != "add" || ops[1].Path != "/metadata/annotations" {
		t.Errorf("annotations op = %+v", ops[1])
	}
	if annotations[kube.AnnotationBaseDigest] != baseDigest {
		t.Errorf("base digest = %q", annotations[kube.AnnotationBaseDigest])
	}
	if annotations[kube.AnnotationBaseVersion] != "1.0" {
		t.Errorf("base version = %q", annotations[kube.AnnotationBaseVersion])
	}
	if annotations[kube.AnnotationManaged] != "true" {
		t.Errorf("managed = %q", annotations[kube.AnnotationManaged])
	}
	if annotations[kube.AnnotationWebhook] != "true" {
		t.Errorf("webhook = %q", annotations[kube.AnnotationWebhook])
	}
}

func TestMutateReplacesExistingAnnotations(t *testing.T) {
	ops, err := newMutator(t, true).Mutate(context.Background(), deployment(oldImage, map[string]string{
		"team":                     "platform",
		kube.AnnotationBaseDigest:  baseDigest,
		kube.AnnotationBaseVersion: "1.0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[1].Op != "replace" {
		t.Fatalf("ops = %+v", ops)
	}
	annotations := ops[1].Value.(map[string]string)
	if annotations["team"] != "platform" {
		t.Errorf("existing annotation lost: %+v", annotations)
	}
}

func TestNoMappingLeavesDeploymentAlone(t *testing.T) {
	ops, err := newMutator(t, false).Mutate(context.Background(), deployment(oldImage, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("ops = %+v", ops)
	}
}

func TestSkipAnnotationWins(t *testing.T) {
	d := deployment(oldImage, map[string]string{kube.AnnotationSkipWebhook: "true"})
	ops, err := newMutator(t, true).Mutate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("ops = %+v", ops)
	}
}

func TestAlreadyAtTargetIsNoOp(t *testing.T) {
	d := deployment(targetImage, map[string]string{
		kube.AnnotationBaseDigest:  baseDigest,
		kube.AnnotationBaseVersion: "1.0",
		kube.AnnotationManaged:     "true",
		kube.AnnotationWebhook:     "true",
	})
	ops, err := newMutator(t, true).Mutate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("ops = %+v", ops)
	}
}

func TestMutateRecognizesBaseByDigestRegardlessOfName(t *testing.T) {
	// The container and Deployment are named "app", not "api", but the image
	// is api's feature build. Recognition is by digest, not by name.
	d := namedDeployment("app", "app", testRepo+"@"+baseDigest, nil)
	ops, err := newMutator(t, true).Mutate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].Op != "replace" || ops[0].Value != targetImage {
		t.Errorf("image patch = %+v", ops[0])
	}
	annotations := ops[1].Value.(map[string]string)
	if annotations[kube.AnnotationBaseDigest] != baseDigest {
		t.Errorf("base digest = %q", annotations[kube.AnnotationBaseDigest])
	}
}

func TestMutateResolvesTagWithoutAllowTags(t *testing.T) {
	// A deployer pins the base by tag. The webhook resolves it to a digest and
	// rewrites it, with AllowTags off, because recognition is digest-first.
	d := deployment(testRepo+":1.0", nil)
	ops, err := newMutator(t, true).Mutate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].Op != "replace" || ops[0].Value != targetImage {
		t.Errorf("image patch = %+v", ops[0])
	}
	annotations := ops[1].Value.(map[string]string)
	if annotations[kube.AnnotationBaseDigest] != baseDigest {
		t.Errorf("base digest = %q", annotations[kube.AnnotationBaseDigest])
	}
}

func TestMutateRecognizesAliasAndRecordsCanonicalBase(t *testing.T) {
	// An older patched image listed as an alias is rewritten to the target,
	// and the recorded base-digest is the canonical feature build, not the alias.
	prefs := catalog.New()
	alias := testRepo + "@" + runningDigest
	if err := prefs.SetMapping("api", catalog.Mapping{
		Base:    testRepo + "@" + baseDigest,
		Aliases: []string{alias},
		Target:  testRepo + "@" + targetDigest,
	}); err != nil {
		t.Fatal(err)
	}
	m := &Mutator{
		Registry:        fakeResolver{},
		LoadPreferences: func() (*catalog.File, error) { return prefs, nil },
	}
	ops, err := m.Mutate(context.Background(), namedDeployment("app", "app", alias, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %+v", ops)
	}
	if ops[0].Value != targetImage {
		t.Errorf("image patch = %+v", ops[0])
	}
	annotations := ops[1].Value.(map[string]string)
	if annotations[kube.AnnotationBaseDigest] != baseDigest {
		t.Errorf("base digest = %q, want canonical %q", annotations[kube.AnnotationBaseDigest], baseDigest)
	}
}

func newMutator(t *testing.T, withMapping bool) *Mutator {
	t.Helper()
	return newMutatorWithTags(t, withMapping, false)
}

func newMutatorWithTags(t *testing.T, withMapping, allowTags bool) *Mutator {
	t.Helper()
	prefs := catalog.New()
	if withMapping {
		if err := prefs.Set("api", testRepo+"@"+baseDigest, testRepo+"@"+targetDigest, ""); err != nil {
			t.Fatal(err)
		}
	}
	return &Mutator{
		Registry:  fakeResolver{digests: map[string]string{testRepo + ":1.0": baseDigest}},
		AllowTags: allowTags,
		LoadPreferences: func() (*catalog.File, error) {
			return prefs, nil
		},
	}
}

func deployment(image string, annotations map[string]string) *appsv1.Deployment {
	return namedDeployment("api", "api", image, annotations)
}

func namedDeployment(name, container, image string, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: container, Image: image}},
				},
			},
		},
	}
}

type fakeResolver struct {
	digests map[string]string
}

func (f fakeResolver) Resolve(_ context.Context, ref string) (string, error) {
	if digest, ok := f.digests[ref]; ok {
		return digest, nil
	}
	return "", fmt.Errorf("no digest for %s", ref)
}
