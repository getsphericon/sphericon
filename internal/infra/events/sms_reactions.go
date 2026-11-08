package events

import "time"

const (
	NameSmsStopObserved           = "sms.stop_observed"
	NameSmsStartObserved          = "sms.start_observed"
	NameSmsDestinationUnreachable = "sms.destination_unreachable"
	NameSmsOptOutRecorded         = "sms.opt_out_recorded"
	NameSmsSuppressionRecorded    = "sms.suppression_recorded"
)

// SmsStopObserved is a semantic stop, not a public Event or a command naming a registry writer.
// Only a link on a received message supplies OutboundMessageID for independent attribution, and
// the Contact the message was sent to and its Broadcast or Automation, which the public
// sms.unsubscribed names when that Contact still holds the number.
type SmsStopObserved struct {
	WorkspaceID       int64     `json:"workspaceId"`
	Phone             string    `json:"phone"`
	Via               string    `json:"via"`
	OutboundMessageID int64     `json:"outboundMessageId,omitempty"`
	ContactID         int64     `json:"contactId,omitempty"`
	BroadcastID       int64     `json:"broadcastId,omitempty"`
	AutomationID      int64     `json:"automationId,omitempty"`
	DedupID           string    `json:"dedupId,omitempty"`
	ObservedAt        time.Time `json:"observedAt,omitzero"`
}

func (*SmsStopObserved) EventName() string            { return NameSmsStopObserved }
func (*SmsStopObserved) EventVersion() int            { return 1 }
func (e *SmsStopObserved) Workspace() int64           { return e.WorkspaceID }
func (e *SmsStopObserved) EventID() string            { return e.DedupID }
func (e *SmsStopObserved) ObservationTime() time.Time { return e.ObservedAt }

// SmsStartObserved gives a START its own source identity before it can lift a later stop.
type SmsStartObserved struct {
	WorkspaceID int64     `json:"workspaceId"`
	Phone       string    `json:"phone"`
	DedupID     string    `json:"dedupId,omitempty"`
	ObservedAt  time.Time `json:"observedAt,omitzero"`
}

func (*SmsStartObserved) EventName() string            { return NameSmsStartObserved }
func (*SmsStartObserved) EventVersion() int            { return 1 }
func (e *SmsStartObserved) Workspace() int64           { return e.WorkspaceID }
func (e *SmsStartObserved) EventID() string            { return e.DedupID }
func (e *SmsStartObserved) ObservationTime() time.Time { return e.ObservedAt }

// SmsDestinationUnreachable carries the adapter's verdict, never an error code to reinterpret.
type SmsDestinationUnreachable struct {
	WorkspaceID int64     `json:"workspaceId"`
	Phone       string    `json:"phone"`
	ContactID   int64     `json:"contactId,omitempty"`
	DedupID     string    `json:"dedupId,omitempty"`
	ObservedAt  time.Time `json:"observedAt,omitzero"`
}

func (*SmsDestinationUnreachable) EventName() string            { return NameSmsDestinationUnreachable }
func (*SmsDestinationUnreachable) EventVersion() int            { return 1 }
func (e *SmsDestinationUnreachable) Workspace() int64           { return e.WorkspaceID }
func (e *SmsDestinationUnreachable) EventID() string            { return e.DedupID }
func (e *SmsDestinationUnreachable) ObservationTime() time.Time { return e.ObservedAt }

// SmsOptOutRecorded is an actual state change, distinct from a repeated stop observation.
type SmsOptOutRecorded struct {
	WorkspaceID int64     `json:"workspaceId"`
	Phone       string    `json:"phone"`
	ObservedAt  time.Time `json:"observedAt"`
}

func (*SmsOptOutRecorded) EventName() string            { return NameSmsOptOutRecorded }
func (*SmsOptOutRecorded) EventVersion() int            { return 1 }
func (e *SmsOptOutRecorded) Workspace() int64           { return e.WorkspaceID }
func (e *SmsOptOutRecorded) ObservationTime() time.Time { return e.ObservedAt }

// SmsSuppressionRecorded is a new suppression, not another report of one already standing.
type SmsSuppressionRecorded SmsOptOutRecorded

func (*SmsSuppressionRecorded) EventName() string            { return NameSmsSuppressionRecorded }
func (*SmsSuppressionRecorded) EventVersion() int            { return 1 }
func (e *SmsSuppressionRecorded) Workspace() int64           { return e.WorkspaceID }
func (e *SmsSuppressionRecorded) ObservationTime() time.Time { return e.ObservedAt }
