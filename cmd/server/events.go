package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// eventsOps is what the `events` commands need from the app. The app implements it
// (internal/app).
type eventsOps interface {
	ReplayDeadLetters(ctx context.Context, group string) (int, error)
}

// eventsCommand is `sphericon events replay <group>`: it runs the domain events a consumer
// group's handler gave up on (its dead letters) through that handler again, oldest first, and
// deletes each one that succeeds. It stops at the first that fails again. Only that group sees
// them: the other groups consumed the events already.
func eventsCommand(d deps) *cli.Command {
	return group("events", "the domain events",
		&cli.Command{
			Name:      "replay",
			Usage:     "run a consumer group's dead letters through its handler again",
			ArgsUsage: "<group>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "group", Required: true}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				group := cmd.StringArg("group")
				n, err := ops.ReplayDeadLetters(ctx, group)
				if _, werr := fmt.Fprintf(out(cmd), "replayed %d dead letters of %s\n", n, group); werr != nil {
					return werr
				}
				return err
			}),
		},
	)
}
