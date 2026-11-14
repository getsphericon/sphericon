package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

var _ = db.Declare(db.Fault("event_journal_entries_event_id_key", "the bus generates a fresh random ULID for each accepted publication"))

var ErrWorkspaceErased = errors.New("events: cannot publish to an erased Workspace")

// ReplayPolicy explicitly retains a consumer's anonymous job input. Its version is the
// retained input's schema, independently of the domain event's version. The owner must
// ensure that input is safe to keep through Contact Erasure and repeated handling converges.
// The event envelope and job metadata are never retained by this policy.
type ReplayPolicy struct {
	kind    string
	version int
	decode  func(json.RawMessage) (river.JobArgs, error)
}

// RetainForReplay opts a consumer into history using its normal, anonymous job args.
// No consumer is replayable by default. The type's Kind is the stable replay selector, and the
// args name their Workspace (jobkind.OfWorkspace), which publish and replay check.
func RetainForReplay[A interface {
	river.JobArgs
	jobkind.OfWorkspace
}](version int) *ReplayPolicy {
	var zero A
	return &ReplayPolicy{kind: zero.Kind(), version: version, decode: func(data json.RawMessage) (river.JobArgs, error) {
		var args A
		if err := json.Unmarshal(data, &args); err != nil {
			return nil, err
		}
		return args, nil
	}}
}

type retainedInput struct {
	Version int             `json:"version"`
	Args    json.RawMessage `json:"args"`
}

func (p *txPublisher) journal(ctx context.Context, env Envelope, inputs map[string]retainedInput) error {
	if env.WorkspaceID != 0 {
		err := holdJournalWorkspace(ctx, p.client, env.WorkspaceID)
		if ent.IsNotFound(err) {
			return ErrWorkspaceErased
		}
		if err != nil {
			return err
		}
	}
	row := p.client.EventJournalEntry.Create().SetEventID(env.ID).SetName(env.Name).
		SetVersion(env.Version).SetOccurredAt(env.OccurredAt).SetRecordedAt(env.RecordedAt)
	if env.WorkspaceID != 0 {
		row.SetWorkspaceID(env.WorkspaceID)
	}
	if len(inputs) != 0 {
		data, err := json.Marshal(inputs)
		if err != nil {
			return fmt.Errorf("encode retained inputs: %w", err)
		}
		row.SetInputs(data)
	}
	if err := row.Exec(ctx); err != nil {
		return fmt.Errorf("journal %q: %w", env.Name, err)
	}
	return nil
}

// holdJournalWorkspace locks the Workspace the journal writes or replays for, until the
// transaction ends, so its erasure's final purge waits for it; a Workspace that is erased, or does
// not meet also, is ent's not found. Key share fences the final FOR UPDATE purge without blocking
// ordinary non-key writes, notably workspacefeed's position update while its Instance-wide
// advisory lock is held.
func holdJournalWorkspace(ctx context.Context, client *ent.Client, id int64, also ...predicate.Workspace) error {
	where := append([]predicate.Workspace{workspace.ID(id), workspace.ErasedAtIsNil()}, also...)
	_, err := client.Workspace.Query().Where(where...).
		ForShare(func(options *sql.LockOptions) { options.Strength = sql.LockKeyShare }).OnlyID(ctx)
	return err
}
