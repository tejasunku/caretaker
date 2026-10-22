// Package health verifies that a rollout actually became healthy. Kubernetes
// supplies the rollout signal; an optional command supplies application-level
// checks (cross-service probes, smoke tests, etc.).
package health

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tejasunku/caretaker/internal/kube"
	"github.com/tejasunku/caretaker/internal/runner"
)

// Checker runs rollout and application checks.
type Checker struct {
	Runner runner.Runner
}

// Verify waits for the rollout, then runs command when non-empty. Placeholders
// {cluster}, {namespace}, and {service} are substituted.
func (c Checker) Verify(ctx context.Context, cl kube.Cluster, service string, timeout time.Duration, command string) error {
	if err := cl.WaitForRollout(ctx, service, timeout); err != nil {
		return err
	}
	if strings.TrimSpace(command) == "" {
		return nil
	}
	expanded := strings.NewReplacer(
		"{cluster}", cl.ID(),
		"{namespace}", cl.Namespace(),
		"{service}", service,
	).Replace(command)
	if err := c.Runner.Run(ctx, "sh", "-c", expanded); err != nil {
		return fmt.Errorf("health check for %s/%s: %w", cl.ID(), service, err)
	}
	return nil
}
