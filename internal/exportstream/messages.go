package exportstream

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// MessagesSchema is the TypeSpec row schema (openapi component) a MessageRow encodes to.
const MessagesSchema = "ExportMessageRow"

// MessageRow is one line of the Outbound messages stream, the contract's ExportMessageRow:
// delivery provenance and outcome, never a subject or a body. Every key is always present,
// an absent value is null, ids are decimal strings like everywhere in /api.
type MessageRow struct {
	ID                int64      `json:"id,string"`
	Kind              string     `json:"kind"`
	Channel           string     `json:"channel"`
	Destination       *string    `json:"destination"`
	Status            string     `json:"status"`
	Reason            *string    `json:"reason"`
	ContactID         *int64     `json:"contact_id,string"`
	BroadcastID       *int64     `json:"broadcast_id,string"`
	AutomationID      *int64     `json:"automation_id,string"`
	AutomationRunID   *int64     `json:"automation_run_id,string"`
	AutomationStep    *int       `json:"automation_step"`
	IntegrationID     *int64     `json:"integration_id,string"`
	TemplateID        *int64     `json:"template_id,string"`
	SendingSource     *string    `json:"sending_source"`
	SendingDomain     *string    `json:"sending_domain"`
	ProviderMessageID *string    `json:"provider_message_id"`
	ClaimedAt         time.Time  `json:"claimed_at"`
	SentAt            *time.Time `json:"sent_at"`
	DeliveredAt       *time.Time `json:"delivered_at"`
	BouncedAt         *time.Time `json:"bounced_at"`
	ComplainedAt      *time.Time `json:"complained_at"`
	Captured          bool       `json:"captured"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// Position is the row's (updated_at, id).
func (r MessageRow) Position() Cursor { return Cursor{At: r.UpdatedAt, ID: r.ID} }

func messageRow(m *ent.OutboundMessage) MessageRow {
	return MessageRow{
		ID:                m.ID,
		Kind:              m.Kind.String(),
		Channel:           m.Channel.String(),
		Destination:       lo.EmptyableToPtr(m.Destination),
		Status:            m.Status.String(),
		Reason:            m.Reason,
		ContactID:         m.ContactID,
		BroadcastID:       m.BroadcastID,
		AutomationID:      m.AutomationID,
		AutomationRunID:   m.AutomationRunID,
		AutomationStep:    m.AutomationStep,
		IntegrationID:     m.IntegrationID,
		TemplateID:        m.TemplateID,
		SendingSource:     m.SendingSource,
		SendingDomain:     m.SendingDomain,
		ProviderMessageID: m.ProviderMessageID,
		ClaimedAt:         m.ClaimedAt.UTC(),
		SentAt:            utc(m.SentAt),
		DeliveredAt:       utc(m.DeliveredAt),
		BouncedAt:         utc(m.BouncedAt),
		ComplainedAt:      utc(m.ComplainedAt),
		Captured:          m.Captured,
		CreatedAt:         m.CreatedAt.UTC(),
		UpdatedAt:         m.UpdatedAt.UTC(),
	}
}

// Messages serves a page of the Workspace's Outbound messages stream.
func Messages(ctx context.Context, s *ent.Scoped, token string, limit int) (Page, error) {
	return Read(token, limit, func(after Cursor, n int) ([]MessageRow, error) {
		rows, err := s.OutboundMessage().Query().
			Where(predicate.OutboundMessage(Seek(after))).
			Order(Order).
			Limit(n).
			All(ctx)
		if err != nil {
			return nil, err
		}
		return lo.Map(rows, func(m *ent.OutboundMessage, _ int) MessageRow { return messageRow(m) }), nil
	})
}
