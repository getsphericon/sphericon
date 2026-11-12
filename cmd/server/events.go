package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/usecase/eventhistory"
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
				history, err := c.EventHistory()
				if err != nil {
					return err
				}
				query := events.JournalQuery{
					Name: cmd.String(flagName), EventID: cmd.String(flagEventID),
					From: cmd.Timestamp(flagFrom), Before: cmd.Timestamp(flagBefore), Limit: cmd.Int(flagLimit),
				}
				of := eventhistory.Of{Instance: cmd.Bool(flagInstance), Workspace: cmd.String(flagWorkspace)}
				page, err := history.List(ctx, of, query)
				if err != nil {
					return err
				}
				return json.NewEncoder(out(cmd)).Encode(page)
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
				history, err := c.EventHistory()
				if err != nil {
					return err
				}
				result, err := history.Replay(ctx, cmd.String(flagWorkspace), events.ReplayRequest{
					Consumer: cmd.String(flagConsumer), From: from, Before: before,
					Limit: cmd.Int(flagLimit), DryRun: cmd.Bool(flagDryRun),
				})
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(out(cmd), "matched %d; enqueued %d\n", result.Matched, result.Enqueued)
				return err
			}),
		})
}
