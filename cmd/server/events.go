package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// eventsCommand is `sphericon events`: `list` inspects the publication metadata one Workspace or
// the Instance retained, and `replay` queues one report consumer's jobs again for the events a
// Workspace recorded in a bounded interval.
func eventsCommand(d deps) *cli.Command {
	return group("events", "inspect publication metadata and reapply retained report inputs",
		&cli.Command{
			Name:  "list",
			Usage: "inspect retained publication metadata as JSON; no payloads or queued work",
			MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{{
				Required: true,
				Flags: [][]cli.Flag{
					{&cli.StringFlag{Name: flagWorkspace, Usage: "Workspace public id"}},
					{&cli.BoolFlag{Name: flagInstance, Usage: "only Instance facts, never Workspace facts"}},
				},
			}},
			Flags: []cli.Flag{
				&cli.StringFlag{Name: flagName, Usage: "exact domain-event name"},
				&cli.StringFlag{Name: flagEventID, Usage: "exact journal event id"},
				&cli.TimestampFlag{Name: flagFrom, Usage: "inclusive recorded time, RFC3339", Config: cli.TimestampConfig{Layouts: []string{time.RFC3339Nano}}},
				&cli.TimestampFlag{Name: flagBefore, Usage: "exclusive recorded time, RFC3339", Config: cli.TimestampConfig{Layouts: []string{time.RFC3339Nano}}},
				&cli.IntFlag{Name: flagLimit, Value: events.DefaultJournalLimit, Usage: "maximum entries in newest-first output; truncated reports omitted matches"},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				history, err := eventHistory(c)
				if err != nil {
					return err
				}
				query := events.JournalQuery{
					Name: cmd.String(flagName), EventID: cmd.String(flagEventID),
					From: cmd.Timestamp(flagFrom), Before: cmd.Timestamp(flagBefore), Limit: cmd.Int(flagLimit),
				}
				of := commandline.JournalOf{Instance: cmd.Bool(flagInstance), Workspace: cmd.String(flagWorkspace)}
				return history.List(ctx, out(cmd), of, query)
			}),
		},
		&cli.Command{
			Name:  "replay",
			Usage: "queue one report consumer for a bounded recorded-time interval; no publication or external actions",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: flagWorkspace, Required: true, Usage: "Workspace public id"},
				&cli.StringFlag{Name: flagConsumer, Required: true, Usage: "event_message_observation or event_broadcast_unsubscribe"},
				&cli.StringFlag{Name: flagFrom, Required: true, Usage: "inclusive recorded time, RFC3339"},
				&cli.StringFlag{Name: flagBefore, Required: true, Usage: "exclusive recorded time, RFC3339"},
				&cli.IntFlag{Name: flagLimit, Value: events.MaxReplayEntries, Usage: "refuse the whole selection when it exceeds this bound"},
				&cli.BoolFlag{Name: flagDryRun, Usage: "validate and count without queuing jobs"},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				from, err := time.Parse(time.RFC3339Nano, cmd.String(flagFrom))
				if err != nil {
					return fmt.Errorf("--from: %w", err)
				}
				before, err := time.Parse(time.RFC3339Nano, cmd.String(flagBefore))
				if err != nil {
					return fmt.Errorf("--before: %w", err)
				}
				history, err := eventHistory(c)
				if err != nil {
					return err
				}
				return history.Replay(ctx, out(cmd), cmd.String(flagWorkspace), events.ReplayRequest{
					Consumer: cmd.String(flagConsumer), From: from, Before: before,
					Limit: cmd.Int(flagLimit), DryRun: cmd.Bool(flagDryRun),
				})
			}),
		})
}

// eventHistory is what `sphericon events` reads and replays through: the Workspaces of the command
// app, which resolve the id an operator names, and its bus, which keeps the journal.
func eventHistory(c core) (commandline.EventHistory, error) {
	ws, err := c.Workspaces()
	if err != nil {
		return commandline.EventHistory{}, err
	}
	bus, err := c.Bus()
	if err != nil {
		return commandline.EventHistory{}, err
	}
	return commandline.EventHistory{Workspaces: ws, Bus: bus}, nil
}
