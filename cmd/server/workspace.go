package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/suspension"
)

// workspaceOps is what the `workspace` staff member commands need from the app. The
// app implements it (internal/app); tests use a fake.
type workspaceOps interface {
	SuspendWorkspace(ctx context.Context, slug string, by suspension.Actor, reason string) (bool, error)
	UnsuspendWorkspace(ctx context.Context, slug string, by suspension.Actor) (bool, error)
}

// workspaceCommand is `sphericon workspace suspend|unsuspend` (ADR 0007): the bare
// staff member toggle for a Workspace's outbound-sending freeze. Everything above this
// mechanism (the automated detector, a Staff console) is EE.
func workspaceCommand(d deps) *cli.Command {
	return group("workspace", "freeze or resume a Workspace's outbound sending",
		&cli.Command{
			Name:      "suspend",
			Usage:     "freeze outbound sending; the owner is told the reason",
			ArgsUsage: "<slug> <reason...>",
			Arguments: []cli.Argument{
				&cli.StringArg{Name: "slug", Required: true},
				&cli.StringArgs{Name: "reason", Min: 1, Max: -1},
			},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				slug := cmd.StringArg("slug")
				reason := strings.TrimSpace(strings.Join(cmd.StringArgs("reason"), " "))
				if reason == "" {
					return fmt.Errorf("a reason is required: the owner is told why")
				}
				changed, err := ops.SuspendWorkspace(ctx, slug, suspension.CLI, reason)
				if err != nil {
					return err
				}
				if !changed {
					_, _ = fmt.Fprintf(out(cmd), "workspace %q is already suspended; nothing changed\n", slug)
					return nil
				}
				_, _ = fmt.Fprintf(out(cmd), "workspace %q suspended: outbound sending is frozen and the owner has been notified\n", slug)
				return nil
			}),
		},
		&cli.Command{
			Name:      "unsuspend",
			Usage:     "resume outbound sending",
			ArgsUsage: "<slug>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "slug", Required: true}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				slug := cmd.StringArg("slug")
				changed, err := ops.UnsuspendWorkspace(ctx, slug, suspension.CLI)
				if err != nil {
					return err
				}
				if !changed {
					_, _ = fmt.Fprintf(out(cmd), "workspace %q is not suspended; nothing changed\n", slug)
					return nil
				}
				_, _ = fmt.Fprintf(out(cmd), "workspace %q unsuspended: held sends will resume\n", slug)
				return nil
			}),
		},
	)
}
