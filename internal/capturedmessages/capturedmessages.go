// Package capturedmessages is the store of Captured messages (ADR 0058): what a Test inbox
// kept instead of sending it out. It is the one place their rows are written (review rule 5);
// the Test inbox provider reaches it through the messaging.Capturer this module binds to a
// Workspace and an Integration, and the site API reads it through List.
//
// Every method takes the Workspace's *ent.Scoped, so a Workspace only ever sees its own.
package capturedmessages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Module holds the one setting a capture needs: how many Captured messages a Workspace keeps. The
// Workspace comes with each call.
type Module struct {
	kept int
}

// New builds the module; a Workspace keeps its newest `kept` Captured messages (at least one).
func New(kept int) *Module { return &Module{kept: max(kept, 1)} }

// Capturer is the messaging.Capturer of one Workspace's Test inbox: what it is sent is kept as
// a Captured message owned by that Workspace and Integration.
func (m *Module) Capturer(s *ent.Scoped, integrationID int64) messaging.Capturer {
	return capturer{s: s, integrationID: integrationID, kept: m.kept}
}

type capturer struct {
	s             *ent.Scoped
	integrationID int64
	kept          int
}

// Capture keeps the message. The From is flagged when it is not on a verified Sending domain of
// the Workspace, the way the gate at a provider that delivers outside would refuse it.
func (c capturer) Capture(ctx context.Context, captured messaging.Captured) error {
	msg := captured.Message
	verified, err := messaging.HasVerifiedSendingDomain(ctx, c.s, msg.From)
	if err != nil {
		return fmt.Errorf("capturedmessages: check sending domain: %w", err)
	}
	origin := lo.FromPtr(msg.Origin)
	surface := capturedmessage.Surface(origin.Surface)
	if origin.Surface == "" {
		surface = capturedmessage.SurfaceTest
	}
	create := c.s.CapturedMessage().Create().
		SetIntegrationID(c.integrationID).
		SetSurface(surface).
		SetRecipient(suppression.NormalizeDestination(msg.To)).
		SetFromEmail(msg.From).
		SetSubject(msg.Subject).
		SetHTML(msg.HTML).
		SetText(msg.Text).
		SetHeaders(lo.Map(captured.Headers, func(h messaging.Header, _ int) schema.CapturedHeader {
			return schema.CapturedHeader{Name: h.Name, Value: h.Value}
		})).
		SetLinks(captured.Links).
		SetFromUnverified(!verified)
	if msg.FromName != "" {
		create.SetFromName(msg.FromName)
	}
	if origin.Source != "" {
		create.SetSendingSource(origin.Source)
	}
	if origin.OutboundMessageID != 0 {
		create.SetOutboundMessageID(origin.OutboundMessageID)
	}
	if err := create.Exec(ctx); err != nil {
		return fmt.Errorf("capturedmessages: keep message: %w", err)
	}
	return c.trim(ctx)
}

// trim deletes what lies beyond the newest `kept` messages of the Workspace. It runs after the
// insert has committed, as a statement of its own: concurrent captures each insert and then trim, and
// whichever trim runs last started after every insert had committed, so it sees them all and the
// Workspace ends at the cap. (Holding a lock instead would serialize every send through a Test inbox.)
// A capture inside a wider transaction would lose that guarantee, so none runs in one.
func (c capturer) trim(ctx context.Context) error {
	edge, err := c.s.CapturedMessage().Query().
		Order(ent.Desc(capturedmessage.FieldCreatedAt), ent.Desc(capturedmessage.FieldID)).
		Offset(c.kept - 1).First(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capturedmessages: find the cap: %w", err)
	}
	// The kept-th newest message is the oldest one kept: everything sorting before it goes.
	_, err = c.s.CapturedMessage().Delete().Where(capturedmessage.Or(
		capturedmessage.CreatedAtLT(edge.CreatedAt),
		capturedmessage.And(capturedmessage.CreatedAt(edge.CreatedAt), capturedmessage.IDLT(edge.ID)),
	)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("capturedmessages: trim to the newest %d: %w", c.kept, err)
	}
	return nil
}

// Purge deletes the Captured messages of every Workspace that are older than retention, in batches
// of batch rows so a large backlog never holds a long lock, and returns how many went. It is
// Instance-wide, so it takes the raw client; a retention of zero or less deletes nothing.
func Purge(ctx context.Context, client *ent.Client, retention time.Duration, batch int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention)
	var total int64
	for {
		ids, err := client.CapturedMessage.Query().
			Where(capturedmessage.CreatedAtLT(cutoff)).
			Order(capturedmessage.ByID()).
			Limit(batch).
			IDs(ctx)
		if err != nil {
			return total, fmt.Errorf("capturedmessages: find expired: %w", err)
		}
		if len(ids) == 0 {
			return total, nil
		}
		n, err := client.CapturedMessage.Delete().Where(capturedmessage.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return total, fmt.Errorf("capturedmessages: purge expired: %w", err)
		}
		total += int64(n)
	}
}

// ErrNotFound reports a Captured message that does not exist in the Workspace.
var ErrNotFound = errors.New("captured message not found")

// Get returns one Captured message of the Workspace, or ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.CapturedMessage, error) {
	row, err := s.CapturedMessage().Query().Where(capturedmessage.ID(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("capturedmessages: get: %w", err)
	}
	return row, nil
}

// Delete removes one Captured message of the Workspace, or reports ErrNotFound.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	n, err := s.CapturedMessage().Delete().Where(capturedmessage.ID(id)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("capturedmessages: delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAll removes every Captured message of the Workspace.
func (m *Module) DeleteAll(ctx context.Context, s *ent.Scoped) error {
	if _, err := s.CapturedMessage().Delete().Exec(ctx); err != nil {
		return fmt.Errorf("capturedmessages: delete all: %w", err)
	}
	return nil
}

// List returns one page of the Workspace's Captured messages, newest first.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.CapturedMessage], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.CapturedMessage().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.CapturedMessage, error) {
			return s.CapturedMessage().Query().
				Order(ent.Desc(capturedmessage.FieldCreatedAt), ent.Desc(capturedmessage.FieldID)).
				Limit(limit).Offset(offset).All(ctx)
		})
}
