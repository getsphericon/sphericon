package events

import (
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The ways a Consent observation went (ConsentSuperseded.Observation).
const (
	// ObservationStop stopped consent: a STOP, a block, a button, a link, a refused send.
	ObservationStop = "stop"
	// ObservationStart gave it again: a START, an unblock, a bot started.
	ObservationStart = "start"
)

// ConsentSuperseded is a Consent observation of a Contact that occurred before the consent state
// standing, and so changed nothing (contact.consent_superseded, ADR 0154): a START delivered after
// a STOP the person sent later, a stop older than the return that lifted it. It is the observation's
// place in the Contact's consent history, at the time it occurred; none of the reactions to a
// transition run for it, and it names no Broadcast. Channel is the observation's, Observation is
// ObservationStop or ObservationStart, Via how a stop came (one of UnsubscribedBy*, "" for a
// start). Phone is the number of an SMS observation. DedupID derives from the observation's, so a
// redelivery records nothing twice.
type ConsentSuperseded struct {
	WorkspaceID   int64     `json:"workspaceId"`
	ContactID     int64     `json:"contactId"`
	Email         string    `json:"email,omitempty"`
	Phone         string    `json:"phone,omitempty"`
	Channel       string    `json:"channel"`
	Observation   string    `json:"observation"`
	Via           string    `json:"via,omitempty"`
	IntegrationID int64     `json:"integrationId,omitempty"`
	DedupID       string    `json:"dedupId,omitempty"`
	ObservedAt    time.Time `json:"observedAt"`
}

func (*ConsentSuperseded) EventName() string            { return systemevent.ContactConsentSuperseded }
func (*ConsentSuperseded) EventVersion() int            { return 1 }
func (e *ConsentSuperseded) Workspace() int64           { return e.WorkspaceID }
func (e *ConsentSuperseded) EventID() string            { return e.DedupID }
func (e *ConsentSuperseded) ObservationTime() time.Time { return e.ObservedAt }
func (e *ConsentSuperseded) Project() Projection {
	props := map[string]any{"channel": e.Channel, "observation": e.Observation}
	if e.Via != "" {
		props["via"] = e.Via
	}
	setNonZero(props, "integrationId", e.IntegrationID)
	return Projection{
		Subject: e.Email, Action: systemevent.ContactConsentSuperseded, Email: e.Email, Phone: e.Phone, ContactID: e.ContactID,
		OccurredAt: e.ObservedAt, Properties: props,
	}
}
