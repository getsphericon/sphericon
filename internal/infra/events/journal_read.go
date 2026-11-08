package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventjournalentry"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// Journal read limits bound operator output independently of replay and API pagination.
const (
	// DefaultJournalLimit bounds output when the operator omits a limit.
	// contract-copy: the operator inspection default is independent of API pagination bounds.
	DefaultJournalLimit = 100
	// MaxJournalLimit bounds one inspection snapshot.
	// contract-copy: the operator inspection cap is independent of API pagination bounds.
	MaxJournalLimit = 1000
)

// JournalQuery filters publication metadata by exact name/id and optional recorded-time bounds.
// From is inclusive and Before exclusive. Zero bounds are open; zero Limit uses the default.
type JournalQuery struct {
	Name    string
	EventID string
	From    time.Time
	Before  time.Time
	Limit   int
}

// JournalEntry exposes metadata and retained consumer names, never their inputs or an envelope.
// RetainedConsumers describes stored history, not a promise that a consumer is currently enabled.
type JournalEntry struct {
	EventID           string    `json:"eventId"`
	Name              string    `json:"name"`
	Version           int       `json:"version"`
	OccurredAt        time.Time `json:"occurredAt"`
	RecordedAt        time.Time `json:"recordedAt"`
	RetainedConsumers []string  `json:"retainedConsumers"`
}

// JournalPage is newest recorded time first, with insertion id breaking ties. Truncated means
// more entries matched the snapshot: narrow the filters or raise the limit, up to its bound.
// This is an inspection snapshot, not a cursor or a promise of publication commit order.
type JournalPage struct {
	Entries   []JournalEntry `json:"entries"`
	Truncated bool           `json:"truncated"`
}

// WorkspaceJournal reads only the named Workspace, including while it awaits deletion.
func (b *Bus) WorkspaceJournal(ctx context.Context, s *ent.Scoped, in JournalQuery) (JournalPage, error) {
	return b.readJournal(ctx, eventjournalentry.WorkspaceID(s.WorkspaceID()), in)
}

// InstanceJournal reads only facts belonging to no Workspace. It never reads all Workspaces.
func (b *Bus) InstanceJournal(ctx context.Context, in JournalQuery) (JournalPage, error) {
	return b.readJournal(ctx, eventjournalentry.WorkspaceIDIsNil(), in)
}

func (b *Bus) readJournal(ctx context.Context, scope predicate.EventJournalEntry, in JournalQuery) (JournalPage, error) {
	page := JournalPage{Entries: []JournalEntry{}}
	if in.Limit == 0 {
		in.Limit = DefaultJournalLimit
	}
	if in.Limit < 1 || in.Limit > MaxJournalLimit {
		return page, fmt.Errorf("events: journal limit must be between 1 and %d", MaxJournalLimit)
	}
	if !in.From.IsZero() && !in.Before.IsZero() && !in.From.Before(in.Before) {
		return page, errors.New("events: journal recorded-time interval must be nonempty")
	}
	err := b.within(ctx, func(client *ent.Client, _ *txPublisher) error {
		query := client.EventJournalEntry.Query().Where(scope)
		if in.Name != "" {
			query.Where(eventjournalentry.Name(in.Name))
		}
		if in.EventID != "" {
			query.Where(eventjournalentry.EventID(in.EventID))
		}
		if !in.From.IsZero() {
			query.Where(eventjournalentry.RecordedAtGTE(in.From))
		}
		if !in.Before.IsZero() {
			query.Where(eventjournalentry.RecordedAtLT(in.Before))
		}
		rows, err := query.Order(eventjournalentry.ByRecordedAt(sql.OrderDesc()), eventjournalentry.ByID(sql.OrderDesc())).
			Limit(in.Limit + 1).All(ctx)
		if err != nil {
			return err
		}
		page.Truncated = len(rows) > in.Limit
		if page.Truncated {
			rows = rows[:in.Limit]
		}
		for _, row := range rows {
			var inputs map[string]json.RawMessage
			if len(row.Inputs) != 0 {
				if err := json.Unmarshal(row.Inputs, &inputs); err != nil {
					return fmt.Errorf("read journal metadata of %s: %w", row.EventID, err)
				}
			}
			consumers := make([]string, 0, len(inputs))
			for kind := range inputs {
				consumers = append(consumers, kind)
			}
			slices.Sort(consumers)
			page.Entries = append(page.Entries, JournalEntry{
				EventID: row.EventID, Name: row.Name, Version: row.Version,
				OccurredAt: row.OccurredAt, RecordedAt: row.RecordedAt, RetainedConsumers: consumers,
			})
		}
		return nil
	})
	if err != nil {
		return JournalPage{}, err
	}
	return page, nil
}
