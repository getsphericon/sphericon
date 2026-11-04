package events

import (
	"context"
	"unicode/utf8"
)

// The System events of what happens to a Contact rather than what it does (#897): its Tags, its
// Custom fields and its Enrollments. Each is published in the transaction of the change it
// records, and only for a real change, so a Trigger, a Rule or a Segment that names one sees each
// change once. All of them are analytical: Retention deletes them by age (ADR 0019).

// ContactTagChange is a Tag applied to a Contact that did not have it (contact.tagged) or removed
// from one that had it (contact.untagged). Tag is the Tag's name.
type ContactTagChange struct {
	Action      string `json:"action"` // contact.tagged|contact.untagged
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId"`
	Email       string `json:"email,omitempty"`
	Tag         string `json:"tag"`
}

func (e *ContactTagChange) EventName() string { return e.Action }
func (*ContactTagChange) EventVersion() int   { return 1 }
func (e *ContactTagChange) Workspace() int64  { return e.WorkspaceID }
func (e *ContactTagChange) Project() Projection {
	return Projection{
		Subject: e.Email, Action: e.Action, Email: e.Email, ContactID: e.ContactID,
		Properties: map[string]any{"tag": e.Tag},
	}
}

// ContactFieldChanged is a Contact's Custom field taking a value it did not hold: set where it had
// none, changed, or cleared. Value is the new value; a cleared field has none, and the old value is
// never recorded.
type ContactFieldChanged struct {
	WorkspaceID int64  `json:"workspaceId"`
	ContactID   int64  `json:"contactId"`
	Email       string `json:"email,omitempty"`
	Key         string `json:"key"`
	Value       any    `json:"value,omitempty"`
}

func (*ContactFieldChanged) EventName() string  { return NameContactFieldChanged }
func (*ContactFieldChanged) EventVersion() int  { return 1 }
func (e *ContactFieldChanged) Workspace() int64 { return e.WorkspaceID }
func (e *ContactFieldChanged) Project() Projection {
	props := map[string]any{"key": e.Key}
	if e.Value != nil {
		props["value"] = e.Value
	}
	return Projection{
		Subject: e.Email, Action: NameContactFieldChanged, Email: e.Email, ContactID: e.ContactID,
		Properties: props,
	}
}

// EnrollmentChange is a Contact's Enrollment in an Automation beginning (automation.enrolled) or
// ending completed, at its goal or exited. The Automation is the Event's automation_id, and its
// `automationId` property too, which a Rule's trigger leaf and the feeds read.
type EnrollmentChange struct {
	Action       string `json:"action"` // automation.enrolled|completed|goal_reached|exited
	WorkspaceID  int64  `json:"workspaceId"`
	ContactID    int64  `json:"contactId"`
	Email        string `json:"email,omitempty"`
	AutomationID int64  `json:"automationId"`
}

func (e *EnrollmentChange) EventName() string { return e.Action }
func (*EnrollmentChange) EventVersion() int   { return 1 }
func (e *EnrollmentChange) Workspace() int64  { return e.WorkspaceID }
func (e *EnrollmentChange) Project() Projection {
	return Projection{
		Subject: e.Email, Action: e.Action, Email: e.Email, ContactID: e.ContactID,
		AutomationID: e.AutomationID, Properties: map[string]any{"automationId": e.AutomationID},
	}
}

// BindingChange is a Telegram chat becoming a Contact's (telegram.bound) or leaving it for another
// (telegram.unbound), recorded in the transaction of the `/start` that moved it. The Integration
// is the bot, a property `integrationId` a Trigger, a Rule or a Segment may read. DedupID is unique
// per fact so that the same update's two events (one Contact's unbound, another's bound) both stand
// and a redelivery of the update records neither twice.
type BindingChange struct {
	Action        string `json:"action"` // telegram.bound|telegram.unbound
	WorkspaceID   int64  `json:"workspaceId"`
	ContactID     int64  `json:"contactId"`
	Email         string `json:"email,omitempty"`
	IntegrationID int64  `json:"integrationId"`
	DedupID       string `json:"dedupId,omitempty"`
}

func (e *BindingChange) EventName() string { return e.Action }
func (*BindingChange) EventVersion() int   { return 1 }
func (e *BindingChange) Workspace() int64  { return e.WorkspaceID }
func (e *BindingChange) EventID() string   { return e.DedupID }
func (e *BindingChange) Project() Projection {
	return Projection{
		Subject: e.Email, Action: e.Action, Email: e.Email, ContactID: e.ContactID,
		Properties: map[string]any{"integrationId": e.IntegrationID},
	}
}

// MaxReceivedText is the most of a message a received Event (TelegramReceived, SmsReceived)
// keeps, in bytes (1 KiB).
const MaxReceivedText = 1024

// ReceivedText is a message's text as a received Event keeps it: cut at MaxReceivedText bytes,
// never inside a character.
func ReceivedText(s string) string {
	n := MaxReceivedText
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// TelegramReceived is a message a Contact sent a bot from a chat Bound to it (telegram.received),
// recorded in the transaction of the Ingest request. Text is the message cut at MaxReceivedText;
// the Integration is the bot, a property `integrationId` next to `text` that a Trigger, a Rule or
// a Segment may read. DedupID is the update's key, so a redelivery records nothing twice.
type TelegramReceived struct {
	WorkspaceID   int64  `json:"workspaceId"`
	ContactID     int64  `json:"contactId"`
	Email         string `json:"email,omitempty"`
	IntegrationID int64  `json:"integrationId"`
	Text          string `json:"text"`
	DedupID       string `json:"dedupId,omitempty"`
}

func (*TelegramReceived) EventName() string  { return NameTelegramReceived }
func (*TelegramReceived) EventVersion() int  { return 1 }
func (e *TelegramReceived) Workspace() int64 { return e.WorkspaceID }
func (e *TelegramReceived) EventID() string  { return e.DedupID }
func (e *TelegramReceived) Project() Projection {
	return Projection{
		Subject: e.Email, Action: NameTelegramReceived, Email: e.Email, ContactID: e.ContactID,
		Properties: map[string]any{"integrationId": e.IntegrationID, "text": e.Text},
	}
}

// TelegramSent is an Outbound send a bot's Telegram accepted (telegram.sent), recorded with the
// Outbound message in one transaction. Destination is the Telegram user id of the first Binding
// that accepted it; the Broadcast or Automation it came from is named when there is one.
type TelegramSent struct {
	WorkspaceID       int64  `json:"workspaceId"`
	ContactID         int64  `json:"contactId"`
	Email             string `json:"email,omitempty"`
	IntegrationID     int64  `json:"integrationId,omitempty"`
	Destination       string `json:"destination"`
	BroadcastID       int64  `json:"broadcastId,omitempty"`
	AutomationID      int64  `json:"automationId,omitempty"`
	OutboundMessageID int64  `json:"outboundMessageId"`
	Surface           string `json:"surface,omitempty"`
	SendingSource     string `json:"sendingSource,omitempty"`
	DedupID           string `json:"dedupId,omitempty"`
}

func (*TelegramSent) EventName() string  { return NameTelegramSent }
func (*TelegramSent) EventVersion() int  { return 1 }
func (e *TelegramSent) Workspace() int64 { return e.WorkspaceID }
func (e *TelegramSent) EventID() string  { return e.DedupID }
func (e *TelegramSent) Project() Projection {
	props := map[string]any{"destination": e.Destination, "outboundMessageId": e.OutboundMessageID}
	if e.IntegrationID != 0 {
		props["integrationId"] = e.IntegrationID
	}
	if e.BroadcastID != 0 {
		props["broadcastId"] = e.BroadcastID
	}
	if e.AutomationID != 0 {
		props["automationId"] = e.AutomationID
	}
	if e.Surface != "" {
		props["surface"] = e.Surface
	}
	if e.SendingSource != "" {
		props["sendingSource"] = e.SendingSource
	}
	return Projection{
		Subject: e.Email, Action: NameTelegramSent, Email: e.Email, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, Properties: props,
	}
}

// SmsSent is an Outbound send the SMS provider accepted (sms.sent), recorded with the Outbound
// message in one transaction. Destination is the number in E.164; the Broadcast or Automation it
// came from is named when there is one.
type SmsSent struct {
	WorkspaceID       int64  `json:"workspaceId"`
	ContactID         int64  `json:"contactId"`
	Email             string `json:"email,omitempty"`
	IntegrationID     int64  `json:"integrationId,omitempty"`
	Destination       string `json:"destination"`
	BroadcastID       int64  `json:"broadcastId,omitempty"`
	AutomationID      int64  `json:"automationId,omitempty"`
	OutboundMessageID int64  `json:"outboundMessageId"`
	Surface           string `json:"surface,omitempty"`
	SendingSource     string `json:"sendingSource,omitempty"`
	DedupID           string `json:"dedupId,omitempty"`
}

func (*SmsSent) EventName() string  { return NameSmsSent }
func (*SmsSent) EventVersion() int  { return 1 }
func (e *SmsSent) Workspace() int64 { return e.WorkspaceID }
func (e *SmsSent) EventID() string  { return e.DedupID }
func (e *SmsSent) Project() Projection {
	props := map[string]any{"destination": e.Destination, "outboundMessageId": e.OutboundMessageID}
	if e.IntegrationID != 0 {
		props["integrationId"] = e.IntegrationID
	}
	if e.BroadcastID != 0 {
		props["broadcastId"] = e.BroadcastID
	}
	if e.AutomationID != 0 {
		props["automationId"] = e.AutomationID
	}
	if e.Surface != "" {
		props["surface"] = e.Surface
	}
	if e.SendingSource != "" {
		props["sendingSource"] = e.SendingSource
	}
	return Projection{
		Subject: e.Email, Action: NameSmsSent, Email: e.Email, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, Properties: props,
	}
}

// TelegramClicked is a click on a short tracked link of a Telegram message (telegram.clicked, ADR
// 0083), with the URL it led to and the Broadcast or Automation and Outbound message it came from.
// Machine is set on a hit made by software (Telegram's link-preview fetcher among them): it stays
// in the log and counts toward no figure, no rollup, no Trigger and no Attribution.
type TelegramClicked struct {
	WorkspaceID       int64  `json:"workspaceId"`
	ContactID         int64  `json:"contactId"`
	Email             string `json:"email,omitempty"`
	IntegrationID     int64  `json:"integrationId,omitempty"`
	URL               string `json:"url"`
	BroadcastID       int64  `json:"broadcastId,omitempty"`
	AutomationID      int64  `json:"automationId,omitempty"`
	OutboundMessageID int64  `json:"outboundMessageId"`
	Machine           bool   `json:"machine,omitempty"`
	DedupID           string `json:"dedupId,omitempty"`
}

func (*TelegramClicked) EventName() string  { return NameTelegramClicked }
func (*TelegramClicked) EventVersion() int  { return 1 }
func (e *TelegramClicked) Workspace() int64 { return e.WorkspaceID }
func (e *TelegramClicked) EventID() string  { return e.DedupID }
func (e *TelegramClicked) Project() Projection {
	props := map[string]any{"url": e.URL, "outboundMessageId": e.OutboundMessageID}
	if e.IntegrationID != 0 {
		props["integrationId"] = e.IntegrationID
	}
	if e.BroadcastID != 0 {
		props["broadcastId"] = e.BroadcastID
	}
	if e.AutomationID != 0 {
		props["automationId"] = e.AutomationID
	}
	if e.Machine {
		props[MachineProperty] = true
	}
	return Projection{
		Subject: e.Email, Action: NameTelegramClicked, Email: e.Email, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, URL: e.URL, Machine: e.Machine, Properties: props,
	}
}

// Ways a Contact stops Telegram marketing, the `via` property of telegram.unsubscribed.
const (
	// UnsubscribedByKeyword is `/stop` or a STOP keyword.
	UnsubscribedByKeyword = "keyword"
	// UnsubscribedByButton is a press on a message's unsubscribe button.
	UnsubscribedByButton = "button"
	// UnsubscribedByBlock is the person blocking the bot, which Telegram reported by an update or
	// by refusing a send.
	UnsubscribedByBlock = "block"
	// UnsubscribedByLink is a click on the `unsubscribe_url` link of a message, scoped to its source.
	UnsubscribedByLink = "link"
	// UnsubscribedByManual is an opt-out recorded by hand, through the API.
	UnsubscribedByManual = "manual"
)

// TelegramUnsubscribed is a Contact's Channel-wide Telegram opt-out (telegram.unsubscribed), recorded
// with the opt-out in one transaction. Via says how, IntegrationID names the bot it came through,
// and the Broadcast or Automation of the message a button was pressed on is named when it resolves.
// DedupID is unique per Contact, so a redelivery records nothing twice.
type TelegramUnsubscribed struct {
	WorkspaceID   int64  `json:"workspaceId"`
	ContactID     int64  `json:"contactId"`
	Email         string `json:"email,omitempty"`
	IntegrationID int64  `json:"integrationId,omitempty"`
	Via           string `json:"via"`
	BroadcastID   int64  `json:"broadcastId,omitempty"`
	AutomationID  int64  `json:"automationId,omitempty"`
	DedupID       string `json:"dedupId,omitempty"`
}

func (*TelegramUnsubscribed) EventName() string  { return NameTelegramUnsubscribed }
func (*TelegramUnsubscribed) EventVersion() int  { return 1 }
func (e *TelegramUnsubscribed) Workspace() int64 { return e.WorkspaceID }
func (e *TelegramUnsubscribed) EventID() string  { return e.DedupID }
func (e *TelegramUnsubscribed) Project() Projection {
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
		Subject: e.Email, Action: NameTelegramUnsubscribed, Email: e.Email, ContactID: e.ContactID,
		AutomationID: e.AutomationID, BroadcastID: e.BroadcastID, Properties: props,
	}
}

type causeKey struct{}

// CausedBy returns ctx for the work of a Rule's Action: an event published in it carries cause
// as its envelope's CausedBy, and the Rules consumer skips it, so Rules never chain (ADR 0054).
// It is not carried into the jobs the work queues: the steps of an Enrollment a Rule made are the
// Automation's, not the Rule's.
func CausedBy(ctx context.Context, cause string) context.Context {
	return context.WithValue(ctx, causeKey{}, cause)
}

// causeOf is the cause ctx carries, "" outside the work of a Rule's Action.
func causeOf(ctx context.Context) string {
	cause, _ := ctx.Value(causeKey{}).(string)
	return cause
}
