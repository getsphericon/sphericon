package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventjournalentry"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// MaxReplayEntries bounds one atomic replay. A larger selection is refused in full.
// contract-copy: the internal replay batch cap is independent of API pagination bounds.
const MaxReplayEntries = 1000

var (
	ErrReplayConsumerUnsupported  = errors.New("events: consumer has no retained replay input")
	ErrReplayRangeTooLarge        = errors.New("events: replay range exceeds the limit; narrow the interval")
	ErrReplayWorkspaceUnavailable = errors.New("events: replay requires a live Workspace")
)

// ReplayRequest selects one consumer's retained inputs in [From, Before), by recorded time.
// Limit is at most MaxReplayEntries (zero uses that bound). DryRun checks the whole selection
// and its input versions without queuing work. Replay reapplies, it never resets a projection.
type ReplayRequest struct {
	Consumer string
	From     time.Time
	Before   time.Time
	Limit    int
	DryRun   bool
}

type ReplayResult struct {
	Matched  int
	Enqueued int
}

// Replay queues only the named, explicitly replayable consumer's normal jobs. The selection
// comes from one SELECT snapshot; there is no journal cursor and no commit-order assumption.
// It never calls Publish, subscribers or general fan-out. Version, scope, bound and queue
// errors leave no partial work. The Workspace key-share lock fences its final purge until commit.
// The operator names the Workspace by its id (ADR 0043), which its owner resolves
// (internal/usecase/eventhistory): a Workspace that is deleted or erased is
// ErrReplayWorkspaceUnavailable.
func (b *Bus) Replay(ctx context.Context, workspaceID int64, in ReplayRequest) (ReplayResult, error) {
	var result ReplayResult
	if in.From.IsZero() || in.Before.IsZero() || !in.From.Before(in.Before) {
		return result, errors.New("events: replay requires a nonempty recorded-time interval")
	}
	if in.Limit == 0 {
		in.Limit = MaxReplayEntries
	}
	if in.Limit < 1 || in.Limit > MaxReplayEntries {
		return result, fmt.Errorf("events: replay limit must be between 1 and %d", MaxReplayEntries)
	}
	policy, err := b.replayPolicy(in.Consumer)
	if err != nil {
		return result, err
	}
	err = b.within(ctx, func(client *ent.Client, pub *txPublisher) error {
		err := holdJournalWorkspace(ctx, client, workspaceID, workspace.DiscardedAtIsNil())
		if ent.IsNotFound(err) {
			return ErrReplayWorkspaceUnavailable
		}
		if err != nil {
			return err
		}
		rows, err := client.EventJournalEntry.Query().Where(
			eventjournalentry.WorkspaceID(workspaceID),
			eventjournalentry.RecordedAtGTE(in.From), eventjournalentry.RecordedAtLT(in.Before),
			func(selector *sql.Selector) {
				selector.Where(sqljson.HasKey(eventjournalentry.FieldInputs, sqljson.Path(policy.kind)))
			},
		).Order(eventjournalentry.ByRecordedAt(), eventjournalentry.ByID()).Limit(in.Limit + 1).All(ctx)
		if err != nil {
			return err
		}
		if len(rows) > in.Limit {
			return ErrReplayRangeTooLarge
		}
		var params []river.InsertManyParams
		for _, row := range rows {
			var inputs map[string]retainedInput
			if err := json.Unmarshal(row.Inputs, &inputs); err != nil {
				return fmt.Errorf("read retained input of %s: %w", row.EventID, err)
			}
			input := inputs[policy.kind]
			if input.Version != policy.version {
				return fmt.Errorf("events: %s has unsupported input version %d for %q", row.EventID, input.Version, policy.kind)
			}
			args, err := policy.decode(input.Args)
			if err != nil {
				return fmt.Errorf("decode retained input of %s: %w", row.EventID, err)
			}
			var scope jobkind.Workspace
			if err := json.Unmarshal(input.Args, &scope); err != nil {
				return err
			}
			if scope.WorkspaceID != workspaceID || args.Kind() != policy.kind {
				return fmt.Errorf("events: retained input of %s has a different scope or kind", row.EventID)
			}
			params = append(params, river.InsertManyParams{Args: args})
		}
		result.Matched = len(params)
		if in.DryRun || len(params) == 0 {
			return nil
		}
		if pub.jobs == nil {
			return ErrNoJobQueue
		}
		if _, err := pub.jobs.InsertManyTx(ctx, pub.tx, params); err != nil {
			return fmt.Errorf("queue replay: %w", err)
		}
		result.Enqueued = len(params)
		return nil
	})
	if err != nil {
		result.Enqueued = 0
	}
	return result, err
}

func (b *Bus) replayPolicy(kind string) (*ReplayPolicy, error) {
	var found *ReplayPolicy
	for _, c := range b.consumers {
		if c.Replay != nil && c.Replay.kind == kind {
			if found != nil {
				return nil, fmt.Errorf("events: duplicate replay consumer %q", kind)
			}
			found = c.Replay
		}
	}
	if found == nil {
		return nil, fmt.Errorf("%w: %q", ErrReplayConsumerUnsupported, kind)
	}
	return found, nil
}
