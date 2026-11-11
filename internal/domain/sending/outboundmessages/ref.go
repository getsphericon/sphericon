package outboundmessages

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// Ref is the surface record a message is sent for, and through it the message's kind: a
// BroadcastRef, an AutomationRef or a TransactionalRef. The kind is never given apart from the
// record, so a message cannot claim one surface and point at another's; the database checks the
// same rule (outbound_messages_provenance_check).
type Ref interface {
	Kind() outboundmessage.Kind
	// stamp writes the record's ids on a new Outbound message.
	stamp(c *ent.OutboundMessageScopedCreate)
	// owners is the Broadcast and the Automation the message is stamped with on its Events, 0
	// for none.
	owners() (broadcastID, automationID int64)
}

// BroadcastRef is a Broadcast recipient's message.
type BroadcastRef struct {
	BroadcastID int64
	RecipientID int64
}

func (BroadcastRef) Kind() outboundmessage.Kind { return outboundmessage.KindBroadcast }

func (r BroadcastRef) stamp(c *ent.OutboundMessageScopedCreate) {
	c.SetBroadcastID(r.BroadcastID)
	if r.RecipientID != 0 {
		c.SetBroadcastRecipientID(r.RecipientID)
	}
}

func (r BroadcastRef) owners() (broadcastID, automationID int64) { return r.BroadcastID, 0 }

// AutomationRef is the message of the send step at index Step of an Automation run.
type AutomationRef struct {
	AutomationID int64
	RunID        int64
	Step         int
}

func (AutomationRef) Kind() outboundmessage.Kind { return outboundmessage.KindAutomation }

func (r AutomationRef) stamp(c *ent.OutboundMessageScopedCreate) {
	c.SetAutomationID(r.AutomationID).SetAutomationStep(r.Step)
	if r.RunID != 0 {
		c.SetAutomationRunID(r.RunID)
	}
}

func (r AutomationRef) owners() (broadcastID, automationID int64) { return 0, r.AutomationID }

// TransactionalRef is a Transactional message, of the Template it referenced (0 for none).
type TransactionalRef struct {
	TemplateID int64
}

func (TransactionalRef) Kind() outboundmessage.Kind { return outboundmessage.KindTransactional }

func (r TransactionalRef) stamp(c *ent.OutboundMessageScopedCreate) {
	if r.TemplateID != 0 {
		c.SetTemplateID(r.TemplateID)
	}
}

func (TransactionalRef) owners() (broadcastID, automationID int64) { return 0, 0 }
