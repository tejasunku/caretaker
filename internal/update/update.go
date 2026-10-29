// Package update is caretaker's orchestration core: choose a target image,
// record the last known good image, swap in place, verify, and roll back when
// verification fails.
package update

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/tejasunku/caretaker/internal/catalog"
	"github.com/tejasunku/caretaker/internal/health"
	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/registry"
)

// Options controls one update or rollback pass.
type Options struct {
	DryRun        bool
	Verify        bool
	AutoRollback  bool
	Force         bool
	Timeout       time.Duration
	HealthCommand string
	Explicit      map[string]string
	FromLKG       bool
}

// Result is the outcome for one (cluster, service).
type Result struct {
	Cluster string `json:"cluster"`
	Service string `json:"service"`
	From    string `json:"from"`
	To      string `json:"to"`
	Action  string `json:"action"`
	Reason  string `json:"reason,omitempty"`
	Err     error  `json:"-"`
}

// Updater performs updates and rollbacks.
type Updater struct {
	Registry  registry.Registry
	Health    health.Checker
	Now       func() time.Time
	AllowTags bool
}

// LKG is a read-only view of last-known-good state.
type LKG struct {
	Cluster       string
	Service       string
	Current       string
	LastKnownGood string
	Previous      string
	UpdatedAt     string
}

// UpdateCluster updates the selected services in one cluster.
func (u *Updater) UpdateCluster(ctx context.Context, cl kube.Cluster, services []string, prefs *catalog.File, opts Options) []Result {
	deployments, err := cl.ListDeployments(ctx)
	if err != nil {
		return []Result{{Cluster: cl.ID(), Action: "failed", Err: err}}
	}

	wanted := make(map[string]bool, len(services))
	for _, s := range services {
		wanted[s] = true
	}

	results := make([]Result, 0, len(deployments))
	for _, d := range deployments {
		if !wanted[d.Service] {
			continue
		}
		results = append(results, u.updateOne(ctx, cl, d, prefs, opts))
	}
	sortResults(results)
	return results
}

// UpdateAll updates every cluster, continuing past individual failures.
func (u *Updater) UpdateAll(ctx context.Context, clusters []kube.Cluster, services []string, prefs *catalog.File, opts Options) []Result {
	var out []Result
	for _, cl := range clusters {
		out = append(out, u.UpdateCluster(ctx, cl, services, prefs, opts)...)
	}
	return out
}

func (u *Updater) updateOne(ctx context.Context, cl kube.Cluster, d kube.Deployment, prefs *catalog.File, opts Options) Result {
	res := Result{Cluster: cl.ID(), Service: d.Service, From: d.Image, Action: "updated"}

	target, reason, ok := u.chooseTarget(ctx, d, prefs, opts)
	if !ok {
		res.Action = "skipped"
		res.Reason = reason
		res.To = d.Image
		return res
	}
	pinned, err := u.pin(ctx, target)
	if err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}
	res.To = pinned

	targetRef, err := image.Parse(pinned)
	if err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}
	if d.Ref.SameImage(targetRef) {
		res.Action = "skipped"
		res.Reason = "already current"
		return res
	}
	if opts.DryRun {
		res.Action = "would-update"
		return res
	}

	now := u.now()
	currentLKG := d.Annotations[kube.AnnotationLKG]
	annotations := map[string]string{
		kube.AnnotationManaged:    "true",
		kube.AnnotationPrevious:   d.Image,
		kube.AnnotationLastUpdate: now,
	}
	if digest, ok := u.baseDigest(ctx, d); ok {
		annotations[kube.AnnotationBaseDigest] = digest
	}
	if version := u.baseVersion(d); version != "" {
		annotations[kube.AnnotationBaseVersion] = version
	}
	// Seed LKG with the currently running image before the first swap. This is
	// the image we fall back to if verification fails.
	if currentLKG == "" {
		annotations[kube.AnnotationLKG] = d.Image
	}
	if err := cl.Annotate(ctx, d.Service, annotations); err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}

	if err := cl.SetImage(ctx, d.Service, pinned); err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}

	if !opts.Verify {
		return res
	}

	if err := u.Health.Verify(ctx, cl, d.Service, opts.Timeout, opts.HealthCommand); err != nil {
		return u.recover(ctx, cl, d, currentLKG, res, err, opts)
	}

	if err := cl.Annotate(ctx, d.Service, map[string]string{
		kube.AnnotationLKG:   pinned,
		kube.AnnotationLKGAt: now,
	}); err != nil {
		res.Action = "updated-lkg-failed"
		res.Err = err
	}
	return res
}

// recover rolls back to the pre-update image when verification fails.
func (u *Updater) recover(ctx context.Context, cl kube.Cluster, d kube.Deployment, lkg string, res Result, verifyErr error, opts Options) Result {
	res.Err = verifyErr
	if !opts.AutoRollback {
		res.Action = "verify-failed"
		return res
	}
	back := lkg
	if back == "" {
		back = d.Image
	}
	if err := cl.SetImage(ctx, d.Service, back); err != nil {
		res.Action = "rollback-failed"
		res.Err = fmt.Errorf("verify failed: %v; rollback failed: %w", verifyErr, err)
		return res
	}
	if err := cl.WaitForRollout(ctx, d.Service, opts.Timeout); err != nil {
		res.Action = "rollback-failed"
		res.Err = fmt.Errorf("verify failed: %v; rollback did not become healthy: %w", verifyErr, err)
		return res
	}
	res.Action = "rolled-back"
	res.To = back
	return res
}

// chooseTarget resolves the desired image: explicit override, then the
// preferences mapping, then the per-service "newest" policy (the most recently
// added mapping target, not a registry tag scan).
func (u *Updater) chooseTarget(ctx context.Context, d kube.Deployment, prefs *catalog.File, opts Options) (string, string, bool) {
	if opts.Explicit != nil {
		if target, ok := opts.Explicit[d.Service]; ok && target != "" {
			return target, "", true
		}
	}
	if prefs != nil {
		if target, ok := u.Preferred(ctx, d, prefs); ok {
			return target, "", true
		}
		if prefs.Policy(d.Service) == catalog.PolicyNewest {
			if target, ok := prefs.LatestTarget(d.Service); ok {
				return target, "", true
			}
		}
	}
	return "", "no preferred mapping", false
}

// Preferred resolves a digest-only preference: the running image's own digest,
// then the mapping that produced the running patched image, then the recorded
// base digest. Tag fallback is opt-in.
func (u *Updater) Preferred(ctx context.Context, d kube.Deployment, prefs *catalog.File) (string, bool) {
	if prefs == nil {
		return "", false
	}
	if d.Ref.Digest != "" {
		running := d.Ref.Repository + "@" + d.Ref.Digest
		if target, ok := prefs.Lookup(d.Service, running); ok {
			return target, true
		}
		if source, ok := prefs.SourceForTarget(d.Service, running); ok {
			if target, ok := prefs.Lookup(d.Service, source); ok {
				return target, true
			}
		}
	}
	if digest, ok := u.baseDigest(ctx, d); ok {
		if target, ok := prefs.Lookup(d.Service, d.Ref.Repository+"@"+digest); ok {
			return target, true
		}
	}
	return "", false
}

func (u *Updater) baseDigest(ctx context.Context, d kube.Deployment) (string, bool) {
	if digest := d.Annotations[kube.AnnotationBaseDigest]; digest != "" {
		return digest, true
	}
	if d.Ref.Digest != "" {
		return d.Ref.Digest, true
	}
	if u.AllowTags {
		if version := baseVersionTag(d); version != "" {
			digest, err := u.Registry.Resolve(ctx, d.Ref.Repository+":"+version)
			if err == nil {
				return digest, true
			}
		}
	}
	return "", false
}

func (u *Updater) baseVersion(d kube.Deployment) string {
	if v := d.Annotations[kube.AnnotationBaseVersion]; v != "" {
		return v
	}
	if !u.AllowTags {
		return ""
	}
	return baseVersionTag(d)
}

func baseVersionTag(d kube.Deployment) string {
	if d.Ref.Tag == "" {
		return ""
	}
	v, _ := image.BaseVersion(d.Ref.Tag)
	return v
}

// pin turns any reference into repo@digest. Tags are accepted only when tag
// support is explicitly enabled.
func (u *Updater) pin(ctx context.Context, ref string) (string, error) {
	parsed, err := image.Parse(ref)
	if err != nil {
		return "", err
	}
	if parsed.Digest != "" {
		return parsed.Repository + "@" + parsed.Digest, nil
	}
	if !u.AllowTags {
		return "", fmt.Errorf("reference %q is a tag; caretaker is digest-first (use --allow-tags to opt in)", ref)
	}
	digest, err := u.Registry.Resolve(ctx, ref)
	if err != nil {
		return "", err
	}
	return parsed.Repository + "@" + digest, nil
}

// Rollback returns one service to its LKG (or an explicit image).
func (u *Updater) Rollback(ctx context.Context, cl kube.Cluster, service, explicit string, opts Options) Result {
	res := Result{Cluster: cl.ID(), Service: service, Action: "rolled-back"}
	d, err := cl.GetDeployment(ctx, service)
	if err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}
	res.From = d.Image

	target := explicit
	if target == "" {
		if opts.FromLKG {
			target = d.Annotations[kube.AnnotationLKG]
		} else {
			// One step back by default: the image replaced by the last update.
			target = d.Annotations[kube.AnnotationPrevious]
			if target == "" {
				target = d.Annotations[kube.AnnotationLKG]
			}
		}
	}
	if target == "" {
		res.Action = "skipped"
		res.Reason = "no recorded rollback target"
		return res
	}
	pinned, err := u.pin(ctx, target)
	if err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}
	res.To = pinned

	targetRef, err := image.Parse(pinned)
	if err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}
	if d.Ref.SameImage(targetRef) {
		res.Action = "skipped"
		res.Reason = "already at target"
		return res
	}
	if opts.DryRun {
		res.Action = "would-rollback"
		return res
	}

	if err := cl.SetImage(ctx, service, pinned); err != nil {
		res.Action = "failed"
		res.Err = err
		return res
	}
	if err := cl.WaitForRollout(ctx, service, opts.Timeout); err != nil {
		res.Action = "rollback-failed"
		res.Err = err
		return res
	}
	now := u.now()
	if err := cl.Annotate(ctx, service, map[string]string{
		kube.AnnotationLKG:        pinned,
		kube.AnnotationLKGAt:      now,
		kube.AnnotationLastUpdate: now,
	}); err != nil {
		res.Action = "rolled-back-annotation-failed"
		res.Err = err
	}
	return res
}

// RollbackCluster rolls back the selected services that actually exist in one
// cluster.
func (u *Updater) RollbackCluster(ctx context.Context, cl kube.Cluster, services []string, explicit string, opts Options) []Result {
	deployments, err := cl.ListDeployments(ctx)
	if err != nil {
		return []Result{{Cluster: cl.ID(), Action: "failed", Err: err}}
	}
	wanted := make(map[string]bool, len(services))
	for _, s := range services {
		wanted[s] = true
	}
	results := make([]Result, 0, len(deployments))
	for _, d := range deployments {
		if !wanted[d.Service] {
			continue
		}
		results = append(results, u.Rollback(ctx, cl, d.Service, explicit, opts))
	}
	sortResults(results)
	return results
}

// RollbackAll rolls back every cluster.
func (u *Updater) RollbackAll(ctx context.Context, clusters []kube.Cluster, services []string, explicit string, opts Options) []Result {
	var out []Result
	for _, cl := range clusters {
		out = append(out, u.RollbackCluster(ctx, cl, services, explicit, opts)...)
	}
	return out
}

// ListLKG reads last-known-good state live from every cluster.
func (u *Updater) ListLKG(ctx context.Context, clusters []kube.Cluster) ([]LKG, error) {
	var out []LKG
	for _, cl := range clusters {
		deployments, err := cl.ListDeployments(ctx)
		if err != nil {
			return nil, err
		}
		for _, d := range deployments {
			out = append(out, LKG{
				Cluster:       cl.ID(),
				Service:       d.Service,
				Current:       d.Image,
				LastKnownGood: d.Annotations[kube.AnnotationLKG],
				Previous:      d.Annotations[kube.AnnotationPrevious],
				UpdatedAt:     d.Annotations[kube.AnnotationLKGAt],
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cluster != out[j].Cluster {
			return out[i].Cluster < out[j].Cluster
		}
		return out[i].Service < out[j].Service
	})
	return out, nil
}

func (u *Updater) now() string {
	now := time.Now
	if u.Now != nil {
		now = u.Now
	}
	return now().UTC().Format(time.RFC3339)
}

func sortResults(results []Result) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Service < results[j].Service
	})
}
