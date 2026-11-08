package main

import (
	"context"
	"errors"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
)

// workspaceCommand is `sphericon workspace suspend|unsuspend` (ADR 0007): the bare
// staff member toggle for a Workspace's outbound-sending freeze. Everything above this
// mechanism (the automated detector, a Staff console) is EE.
func workspaceCommand(d deps) *cli.Command {
	return group("workspace", "freeze or resume a Workspace's outbound sending",
		&cli.Command{
			Name:      "suspend",
			Usage:     "freeze outbound sending; the owner is told the reason",
			ArgsUsage: "<id> <reason...>",
			Arguments: []cli.Argument{
				&cli.StringArg{Name: "id", Required: true},
				&cli.StringArgs{Name: "reason", Min: 1, Max: -1},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				id := cmd.StringArg("id")
				reason := strings.TrimSpace(strings.Join(cmd.StringArgs("reason"), " "))
				if reason == "" {
					return errors.New("a reason is required: the owner is told why")
				}
				suspender, err := c.Suspender()
				if err != nil {
					return err
				}
				changed, err := suspender.SuspendByPublicID(ctx, id, suspension.CLI, reason)
				return commandline.ReportSuspended(out(cmd), id, changed, err)
			}),
		},
		&cli.Command{
			Name:      "unsuspend",
			Usage:     "resume outbound sending",
			ArgsUsage: "<id>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "id", Required: true}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				suspender, err := c.Suspender()
				if err != nil {
					return err
				}
				id := cmd.StringArg("id")
				changed, err := suspender.UnsuspendByPublicID(ctx, id, suspension.CLI)
				return commandline.ReportUnsuspended(out(cmd), id, changed, err)
			}),
		},
	)
}
