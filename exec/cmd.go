package exec

import (
	"bytes"
	"context"
	"errors"
	"io"
)

// CommandExecutor is implemented by execution provider adapters.
// They receive a fully configured Command and must:
//
//   - respect Args / Env / Dir
//   - read from Stdin if provided
//   - write to Stdout / Stderr if provided
//
// Providers do NOT handle capturing or fancy IO — they simply write to whatever
// writer is assigned.
type CommandExecutor interface {
	RunCommand(ctx context.Context, cmd Command) error
}

// Command is a generic DTO describing "run this command somewhere".
// It carries both the execution parameters and the provider that will execute it.
//
// It is implementation-agnostic: local shell, Docker, remote executor, kubectl, etc.
type Command struct {
	exec CommandExecutor

	Args []string

	Env []string
	Dir string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewCommand constructs a command bound to a specific provider
// and initializes it with the required command-line arguments.
//
// Example:
//
//	cmd := exec.NewCommand(execProvider, "gcloud", "config", "list")
//	out, err := cmd.Output(ctx)
func NewCommand(provider CommandExecutor, args ...string) *Command {
	return &Command{
		exec: provider,
		Args: args,
	}
}

// --- Fluent configuration methods ---

// WithArgs replaces the argument list.
func (c *Command) WithArgs(args ...string) *Command {
	c.Args = append([]string(nil), args...)
	return c
}

func (c *Command) WithEnv(env []string) *Command {
	c.Env = env
	return c
}

func (c *Command) WithDir(dir string) *Command {
	c.Dir = dir
	return c
}

func (c *Command) WithStdin(r io.Reader) *Command {
	c.Stdin = r
	return c
}

func (c *Command) WithStdout(w io.Writer) *Command {
	c.Stdout = w
	return c
}

func (c *Command) WithStderr(w io.Writer) *Command {
	c.Stderr = w
	return c
}

// --- Execution ---

// Run executes the command using its bound provider.
func (c *Command) Run(ctx context.Context) error {
	if c.exec == nil {
		return errors.New("exec.Command: no CommandExecutor configured")
	}
	cmd := *c
	return c.exec.RunCommand(ctx, cmd)
}

// Output executes the command and returns stdout as []byte.
func (c *Command) Output(ctx context.Context) ([]byte, error) {
	if c.exec == nil {
		return nil, errors.New("exec.Command: no CommandExecutor configured")
	}

	var buf bytes.Buffer
	cmd := *c

	if cmd.Stdout == nil {
		cmd.Stdout = &buf
	} else {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, &buf)
	}

	if err := c.exec.RunCommand(ctx, cmd); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- Convenience helpers for callers that don't need advanced features ---

func Run(ctx context.Context, provider CommandExecutor, args ...string) error {
	return NewCommand(provider, args...).Run(ctx)
}

func Output(ctx context.Context, provider CommandExecutor, args ...string) ([]byte, error) {
	return NewCommand(provider, args...).Output(ctx)
}
