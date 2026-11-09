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
					{&cli.StringFlag{Name: "workspace", Usage: "Workspace public id"}},
					{&cli.BoolFlag{Name: "instance", Usage: "only Instance facts, never Workspace facts"}},
				},
			}},
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "name", Usage: "exact domain-event name"},
				&cli.StringFlag{Name: "event-id", Usage: "exact journal event id"},
				&cli.TimestampFlag{Name: "from", Usage: "inclusive recorded time, RFC3339", Config: cli.TimestampConfig{Layouts: []string{time.RFC3339Nano}}},
				&cli.TimestampFlag{Name: "before", Usage: "exclusive recorded time, RFC3339", Config: cli.TimestampConfig{Layouts: []string{time.RFC3339Nano}}},
				&cli.IntFlag{Name: "limit", Value: events.DefaultJournalLimit, Usage: "maximum entries in newest-first output; truncated reports omitted matches"},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				history, err := c.EventHistory()
				if err != nil {
					return err
				}
				query := events.JournalQuery{
					Name: cmd.String("name"), EventID: cmd.String("event-id"),
					From: cmd.Timestamp("from"), Before: cmd.Timestamp("before"), Limit: cmd.Int("limit"),
				}
				of := eventhistory.Of{Instance: cmd.Bool("instance"), Workspace: cmd.String("workspace")}
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
				history, err := c.EventHistory()
				if err != nil {
					return err
				}
				result, err := history.Replay(ctx, cmd.String("workspace"), events.ReplayRequest{
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
