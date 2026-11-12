package exportstream

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/systemevent"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
)

// EventsSchema is the TypeSpec row schema (openapi component) an EventRow encodes to.
const EventsSchema = "ExportEventRow"

// EventRow is one line of the Events stream, the contract's ExportEventRow: every key is
// always present, an absent value is null, ids are decimal strings like everywhere in /api.
type EventRow struct {
	ID         publicid.ID    `json:"id"`
	Action     string         `json:"action"`
	Properties map[string]any `json:"properties"`
	ContactID  *publicid.ID   `json:"contact_id"`
	VisitorID  *string        `json:"visitor_id"`
	SubjectID  string         `json:"subject_id"`
	Email      *string        `json:"email"`
	Phone      *string        `json:"phone"`
	OccurredAt time.Time      `json:"occurred_at"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Position is the row's (updated_at, id).
func (r EventRow) Position() Cursor { return Cursor{At: r.UpdatedAt, ID: int64(r.ID)} }

func eventRow(e *ent.Event) EventRow {
	return EventRow{
		ID:         publicid.ID(e.ID),
		Action:     e.Action,
		Properties: lo.Ternary(e.Properties == nil, map[string]any{}, systemevent.PublicProperties(e.Action, e.Properties)),
		ContactID:  publicid.Ptr(e.ContactID),
		VisitorID:  e.VisitorID,
		SubjectID:  e.SubjectID,
		Email:      e.Email,
		Phone:      e.Phone,
		OccurredAt: e.OccurredAt.UTC(),
		CreatedAt:  e.CreatedAt.UTC(),
		UpdatedAt:  e.UpdatedAt.UTC(),
	}
}

// utc is t in UTC, nil staying nil: rows carry one time zone whatever the session had.
func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return new(t.UTC())
}

// Events serves a page of the Workspace's Events stream.
func (st *Streams) Events(ctx context.Context, s *ent.Scoped, token string, limit int) (Page, error) {
	if !st.offered() {
		return Page{}, ErrNotOffered
	}
	return Read(token, limit, func(after Cursor, n int) ([]EventRow, error) {
		rows, err := s.Event().Query().
			Where(predicate.Event(Seek(after))).
			Order(Order).
			Limit(n).
			All(ctx)
		if err != nil {
			return nil, err
		}
		return lo.Map(rows, func(e *ent.Event, _ int) EventRow { return eventRow(e) }), nil
	})
}
