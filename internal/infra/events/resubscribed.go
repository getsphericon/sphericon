package events

import (
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// ResubscribedViaConfirmation is how an email address returned to everything: by confirming a fresh
// confirmation link (ADR 0013). The Preference page adds its own way when it is built (ADR 0121).
const ResubscribedViaConfirmation = "confirmation"

// EmailResubscribed is the person's return to a Sending source of email they had left
// (email.resubscribed): the opt-out of Source is lifted, Via says how. It is evidentiary, kept as
// email.unsubscribed is (systemevent.KeptActions), and occurred at ObservedAt.
type EmailResubscribed struct {
	WorkspaceID int64     `json:"workspaceId"`
	ContactID   int64     `json:"contactId"`
	Email       string    `json:"email"`
	Source      string    `json:"source"`
	Via         string    `json:"via"`
	ObservedAt  time.Time `json:"observedAt"`
}

func (*EmailResubscribed) EventName() string            { return systemevent.EmailResubscribed }
func (*EmailResubscribed) EventVersion() int            { return 1 }
func (e *EmailResubscribed) Workspace() int64           { return e.WorkspaceID }
func (e *EmailResubscribed) ObservationTime() time.Time { return e.ObservedAt }
func (e *EmailResubscribed) Project() Projection {
	return Projection{
		Subject: e.Email, Action: systemevent.EmailResubscribed, Email: e.Email, ContactID: e.ContactID,
		OccurredAt: e.ObservedAt, Properties: map[string]any{"source": e.Source, "via": e.Via},
	}
}
