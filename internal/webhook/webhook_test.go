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

func TestMutateResolvesBaseVersion(t *testing.T) {
	ops, err := newMutator(t, true).Mutate(context.Background(), deployment(oldImage, nil))
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
	ops, err := newMutator(t, true).Mutate(context.Background(), deployment(oldImage, map[string]string{"team": "platform"}))
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
	d := deployment(targetImage, map[string]string{kube.AnnotationBaseVersion: "1.0"})
	ops, err := newMutator(t, true).Mutate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("ops = %+v", ops)
	}
}

func newMutator(t *testing.T, withMapping bool) *Mutator {
	t.Helper()
	prefs := catalog.New()
	if withMapping {
		if err := prefs.Set("api", testRepo+"@"+baseDigest, testRepo+"@"+targetDigest, ""); err != nil {
			t.Fatal(err)
		}
	}
	return &Mutator{
		Registry: fakeResolver{digests: map[string]string{testRepo + ":1.0": baseDigest}},
		LoadPreferences: func() (*catalog.File, error) {
			return prefs, nil
		},
	}
}

func deployment(image string, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Annotations: annotations},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "api", Image: image}},
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
