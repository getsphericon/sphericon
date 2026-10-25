// Package capturedmessages is the store of Captured messages (ADR 0058): what a Test inbox
// kept instead of sending it out. It is the one place their rows are written (review rule 5);
// the Test inbox provider reaches it through the messaging.Capturer this module binds to a
// Workspace and an Integration, and the site API reads it through List.
//
// Every method takes the Workspace's *ent.Scoped, so a Workspace only ever sees its own.
package capturedmessages

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Module is stateless: the Workspace comes with each call.
type Module struct{}

// New builds the module.
func New() *Module { return &Module{} }

// Capturer is the messaging.Capturer of one Workspace's Test inbox: what it is sent is kept as
// a Captured message owned by that Workspace and Integration.
func (m *Module) Capturer(s *ent.Scoped, integrationID int64) messaging.Capturer {
	return capturer{s: s, integrationID: integrationID}
}

type capturer struct {
	s             *ent.Scoped
	integrationID int64
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
