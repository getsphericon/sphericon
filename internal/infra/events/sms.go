package events

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

func (*SmsReceived) EventName() string  { return NameSmsReceived }
func (*SmsReceived) EventVersion() int  { return 1 }
func (e *SmsReceived) Workspace() int64 { return e.WorkspaceID }
func (e *SmsReceived) EventID() string  { return e.DedupID }
func (e *SmsReceived) Project() Projection {
	return Projection{
		Subject: e.Email, Action: NameSmsReceived, Email: e.Email, Phone: e.Phone, ContactID: e.ContactID,
		Properties: map[string]any{"integrationId": e.IntegrationID, "text": e.Text},
	}
}

// SmsUnsubscribed is a Contact's phone number opting out of all of the Workspace's texts
// (sms.unsubscribed), recorded with the opt-out in one transaction, once per new opt-out whatever
// its source. Via says how (UnsubscribedByKeyword, ByLink, ByRefusedSend or ByManual), IntegrationID
// names the sender it came through when there is one, and the Broadcast or Automation of the text it
// was made from is named on the event of the text's Contact. DedupID is unique per opt-out and
// Contact, so a redelivery records nothing twice.
type SmsUnsubscribed struct {
	WorkspaceID   int64  `json:"workspaceId"`
	ContactID     int64  `json:"contactId"`
	Email         string `json:"email,omitempty"`
	Phone         string `json:"phone"`
	IntegrationID int64  `json:"integrationId,omitempty"`
	Via           string `json:"via"`
	BroadcastID   int64  `json:"broadcastId,omitempty"`
	AutomationID  int64  `json:"automationId,omitempty"`
	DedupID       string `json:"dedupId,omitempty"`
}

func (*SmsUnsubscribed) EventName() string  { return NameSmsUnsubscribed }
func (*SmsUnsubscribed) EventVersion() int  { return 1 }
func (e *SmsUnsubscribed) Workspace() int64 { return e.WorkspaceID }
func (e *SmsUnsubscribed) EventID() string  { return e.DedupID }
func (e *SmsUnsubscribed) Project() Projection {
	props := map[string]any{"via": e.Via}
	if e.IntegrationID != 0 {
		props["integrationId"] = e.IntegrationID
	}
	if e.BroadcastID != 0 {
		props["broadcastId"] = e.BroadcastID
	}
	if e.AutomationID != 0 {
		props["automationId"] = e.AutomationID
	}
	return Projection{
		Subject: e.Email, Action: NameSmsUnsubscribed, Email: e.Email, Phone: e.Phone, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, Properties: props,
	}
}
