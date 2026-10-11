package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mokevnin/sphericon/internal/suspension"
)

// workspaceOps is what the `workspace` operator commands need from the app. The
// app implements it (internal/app); tests use a fake.
type workspaceOps interface {
	SuspendWorkspace(ctx context.Context, slug string, by suspension.Actor, reason string) (bool, error)
	UnsuspendWorkspace(ctx context.Context, slug string, by suspension.Actor) (bool, error)
}

const workspaceUsage = "usage: sphericon workspace suspend <slug> <reason...> | unsuspend <slug>"

// runWorkspace executes `sphericon workspace <suspend|unsuspend> …` (ADR 0007): the bare
// operator toggle for a Workspace's outbound-sending freeze. Everything above this
// mechanism (the automated detector, an Operator console) is EE.
func runWorkspace(ctx context.Context, ops workspaceOps, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New(workspaceUsage)
	}
	slug := args[1]
	switch args[0] {
	case "suspend":
		reason := strings.TrimSpace(strings.Join(args[2:], " "))
		if reason == "" {
			return fmt.Errorf("a reason is required: the owner is told why\n%s", workspaceUsage)
		}
		changed, err := ops.SuspendWorkspace(ctx, slug, suspension.CLI, reason)
		if err != nil {
			return err
		}
		if !changed {
			_, _ = fmt.Fprintf(out, "workspace %q is already suspended; nothing changed\n", slug)
			return nil
		}
		_, _ = fmt.Fprintf(out, "workspace %q suspended: outbound sending is frozen and the owner has been notified\n", slug)
	case "unsuspend":
		changed, err := ops.UnsuspendWorkspace(ctx, slug, suspension.CLI)
		if err != nil {
			return err
		}
		if !changed {
			_, _ = fmt.Fprintf(out, "workspace %q is not suspended; nothing changed\n", slug)
			return nil
		}
		_, _ = fmt.Fprintf(out, "workspace %q unsuspended: held sends will resume\n", slug)
	default:
		return fmt.Errorf("unknown workspace command %q\n%s", args[0], workspaceUsage)
	}
	return nil
}
