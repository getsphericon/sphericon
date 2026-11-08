package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/infra/events"
)

// eventsCommand is `sphericon events replay`: it queues one report consumer's jobs again for the
// events a Workspace recorded in a bounded interval.
func eventsCommand(d deps) *cli.Command {
	return group("events", "reapply retained anonymous report inputs",
		&cli.Command{
			Name:  "replay",
			Usage: "queue one report consumer for a bounded recorded-time interval; no publication or external actions",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "workspace", Required: true, Usage: "Workspace public id"},
				&cli.StringFlag{Name: "consumer", Required: true, Usage: "event_message_observation or event_broadcast_unsubscribe"},
				&cli.StringFlag{Name: "from", Required: true, Usage: "inclusive recorded time, RFC3339"},
				&cli.StringFlag{Name: "before", Required: true, Usage: "exclusive recorded time, RFC3339"},
				&cli.IntFlag{Name: "limit", Value: events.MaxReplayEntries, Usage: "refuse the whole selection when it exceeds this bound"},
				&cli.BoolFlag{Name: "dry-run", Usage: "validate and count without queuing jobs"},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				from, err := time.Parse(time.RFC3339Nano, cmd.String("from"))
				if err != nil {
					return fmt.Errorf("--from: %w", err)
				}
				before, err := time.Parse(time.RFC3339Nano, cmd.String("before"))
				if err != nil {
					return fmt.Errorf("--before: %w", err)
				}
				bus, err := c.Bus()
				if err != nil {
					return err
				}
				result, err := bus.Replay(ctx, cmd.String("workspace"), events.ReplayRequest{
					Consumer: cmd.String("consumer"), From: from, Before: before,
					Limit: cmd.Int("limit"), DryRun: cmd.Bool("dry-run"),
				})
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(out(cmd), "matched %d; enqueued %d\n", result.Matched, result.Enqueued)
				return err
			}),
		})
}
