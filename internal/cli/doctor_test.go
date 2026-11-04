package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/update"
)

type doctorFakeRegistry struct{}

func (doctorFakeRegistry) Resolve(context.Context, string) (string, error) { return "", nil }
func (doctorFakeRegistry) Tags(context.Context, string) ([]string, error)  { return nil, nil }
func (doctorFakeRegistry) Exists(context.Context, string) (bool, error)    { return false, nil }
func (doctorFakeRegistry) Catalog(context.Context) ([]string, error)       { return nil, nil }

type doctorFakeLineage struct{}

func (doctorFakeLineage) Metadata(context.Context, string) (lineage.Metadata, bool, error) {
	return lineage.Metadata{}, false, nil
}
func (doctorFakeLineage) Children(context.Context, string) ([]lineage.Child, error) {
	return nil, nil
}
func (doctorFakeLineage) ChildrenForBase(context.Context, string, string) ([]lineage.Child, error) {
	return nil, nil
}

func doctorPrefs(t *testing.T) *catalog.File {
	t.Helper()
	prefs := catalog.New()
	if err := prefs.Set("api", mapTestBase, mapTestNew, ""); err != nil {
		t.Fatal(err)
	}
	return prefs
}

func doctorDeployment(t *testing.T, service, img, baseDigest string) kube.Deployment {
	t.Helper()
	ref, err := image.Parse(img)
	if err != nil {
		t.Fatal(err)
	}
	d := kube.Deployment{Service: service, Container: service, Image: img, Ref: ref, Annotations: map[string]string{}}
	if baseDigest != "" {
		d.Annotations[kube.AnnotationBaseDigest] = baseDigest
	}
	return d
}

func doctorResolver() *update.Updater {
	return &update.Updater{Registry: doctorFakeRegistry{}, Lineage: doctorFakeLineage{}}
}

func TestDoctorResolutionResolvesMappedImage(t *testing.T) {
	prefs := doctorPrefs(t)
	// Running image is the feature build itself.
	d := doctorDeployment(t, "api", mapTestBase, "")
	state, detail, ok := doctorResolution(context.Background(), prefs, doctorResolver(), d)
	if !ok {
		t.Fatal("expected a row for a service with mappings")
	}
	if state != "ok" {
		t.Errorf("state = %q, want ok (%s)", state, detail)
	}
	if !strings.Contains(detail, "prefers") {
		t.Errorf("detail = %q, want the preferred target", detail)
	}
}

// The walkthrough bug: a false base-digest annotation that names nothing in
// the preference map must be surfaced, not silently accepted.
func TestDoctorResolutionWarnsOnFalseBaseDigest(t *testing.T) {
	prefs := doctorPrefs(t)
	bogus := "sha256:" + strings.Repeat("f", 64)
	// Running image is the mapping target (resolves fine) but the annotation
	// points at a digest that is not a feature build.
	d := doctorDeployment(t, "api", mapTestNew, bogus)
	state, detail, ok := doctorResolution(context.Background(), prefs, doctorResolver(), d)
	if !ok {
		t.Fatal("expected a row")
	}
	if state != "warn" {
		t.Errorf("state = %q, want warn (%s)", state, detail)
	}
	if !strings.Contains(detail, "base-digest") {
		t.Errorf("detail = %q, want base-digest complaint", detail)
	}
}

func TestDoctorResolutionWarnsWhenUnresolved(t *testing.T) {
	prefs := doctorPrefs(t)
	unknown := "localhost:5000/test/api@sha256:" + strings.Repeat("9", 64)
	d := doctorDeployment(t, "api", unknown, "")
	state, detail, ok := doctorResolution(context.Background(), prefs, doctorResolver(), d)
	if !ok {
		t.Fatal("expected a row")
	}
	if state != "warn" {
		t.Errorf("state = %q, want warn (%s)", state, detail)
	}
	if !strings.Contains(detail, "no preferred mapping") {
		t.Errorf("detail = %q, want unresolved complaint", detail)
	}
}

func TestDoctorResolutionSkipsServiceWithoutMappings(t *testing.T) {
	d := doctorDeployment(t, "worker", mapTestBase, "")
	if _, _, ok := doctorResolution(context.Background(), catalog.New(), doctorResolver(), d); ok {
		t.Fatal("service without mappings must not produce a row")
	}
}
