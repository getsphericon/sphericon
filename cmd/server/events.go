package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

type eventOps interface {
	ReplayEvents(ctx context.Context, workspace string, in events.ReplayRequest) (events.ReplayResult, error)
	EventJournal(ctx context.Context, in app.EventJournalRequest) (events.JournalPage, error)
}

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
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				page, err := ops.EventJournal(ctx, app.EventJournalRequest{
					Workspace: cmd.String("workspace"), Instance: cmd.Bool("instance"),
					Query: events.JournalQuery{
						Name: cmd.String("name"), EventID: cmd.String("event-id"),
						From: cmd.Timestamp("from"), Before: cmd.Timestamp("before"), Limit: cmd.Int("limit"),
					},
				})
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
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				from, err := time.Parse(time.RFC3339Nano, cmd.String("from"))
				if err != nil {
					return fmt.Errorf("--from: %w", err)
				}
				before, err := time.Parse(time.RFC3339Nano, cmd.String("before"))
				if err != nil {
					return fmt.Errorf("--before: %w", err)
				}
				result, err := ops.ReplayEvents(ctx, cmd.String("workspace"), events.ReplayRequest{
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
