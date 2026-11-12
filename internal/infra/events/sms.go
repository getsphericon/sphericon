package events

import "github.com/getsphericon/sphericon/internal/kernel/systemevent"

// SmsReceived is a text a Contact sent from its phone to an SMS Integration's sender
// (sms.received), recorded in the transaction of the Ingest request. Phone is the sender's number
// in E.164, the Contact's phone; Text is the message cut by ReceivedText; the Integration is the
// one the text reached, a property `integrationId` next to `text` that a Trigger, a Rule or a
// Segment may read. DedupID is the message's key, so a redelivery records nothing twice.
type SmsReceived struct {
	WorkspaceID   int64  `json:"workspaceId"`
	ContactID     int64  `json:"contactId"`
	Email         string `json:"email,omitempty"`
	Phone         string `json:"phone"`
	IntegrationID int64  `json:"integrationId"`
	Text          string `json:"text"`
	DedupID       string `json:"dedupId,omitempty"`
}

func (*SmsReceived) EventName() string  { return systemevent.SmsReceived }
func (*SmsReceived) EventVersion() int  { return 1 }
func (e *SmsReceived) Workspace() int64 { return e.WorkspaceID }
func (e *SmsReceived) EventID() string  { return e.DedupID }
func (e *SmsReceived) Project() Projection {
	return Projection{
		Subject: e.Email, Action: systemevent.SmsReceived, Email: e.Email, Phone: e.Phone, ContactID: e.ContactID,
		Properties: map[string]any{"integrationId": e.IntegrationID, "text": e.Text},
	}
}
