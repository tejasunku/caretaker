// Package runner abstracts external command execution so patch and health
// logic can be tested without spawning copa or trivy.
package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner executes a command.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
}

// Exec runs commands as child processes.
type Exec struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Run implements Runner.
func (e Exec) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = e.Stdout
	cmd.Stderr = e.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// Print is a Runner that records invocations, used in tests.
type Print struct {
	Calls [][]string
}

// Run implements Runner.
func (p *Print) Run(_ context.Context, name string, args ...string) error {
	p.Calls = append(p.Calls, append([]string{name}, args...))
	return nil
}
