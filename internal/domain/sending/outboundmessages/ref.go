package outboundmessages

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// Ref is the surface record a message is sent for, and through it the message's kind and its mode
// of Send-eligibility: a BroadcastRef, an AutomationRef, a TransactionalRef or a ConfirmationRef.
// The kind is never given apart from the record, so a message cannot claim one surface and point at
// another's; the database checks the same rule (outbound_messages_provenance_check). Nor is the mode:
// a new kind of message names its own, or it does not compile (ADR 0013).
type Ref interface {
	Kind() outboundmessage.Kind
	// mode is what Send-eligibility checks the message for; its Sending source, a marketing one's, is
	// the one its unsubscribe link scopes.
	mode() eligibility.Mode
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

// mode: all Broadcasts share one Sending source.
func (BroadcastRef) mode() eligibility.Mode {
	return eligibility.Marketing(eligibility.SourceBroadcasts)
}

// AutomationRef is the message of an Automation's send Step: the version and the Step id that sent
// it (ADR 0122), and the run, 0 for none.
type AutomationRef struct {
	AutomationID int64
	VersionID    int64
	RunID        int64
	StepID       automationstep.ID
}

func (AutomationRef) Kind() outboundmessage.Kind { return outboundmessage.KindAutomation }

func (r AutomationRef) stamp(c *ent.OutboundMessageScopedCreate) {
	c.SetAutomationID(r.AutomationID).SetAutomationVersionID(r.VersionID).SetAutomationStepID(r.StepID)
	if r.RunID != 0 {
		c.SetAutomationRunID(r.RunID)
	}
}

func (r AutomationRef) owners() (broadcastID, automationID int64) { return 0, r.AutomationID }

// mode: each Automation is its own Sending source.
func (r AutomationRef) mode() eligibility.Mode {
	return eligibility.Marketing(eligibility.AutomationSource(r.AutomationID))
}

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

func (TransactionalRef) mode() eligibility.Mode { return eligibility.Transactional() }
