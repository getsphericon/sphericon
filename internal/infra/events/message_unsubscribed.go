package events

import "time"

// NameMessageUnsubscribed is internal attribution to a received message, never a public Event.
const NameMessageUnsubscribed = "message.unsubscribed"

// MessageUnsubscribed carries only anonymous accounting data. Its delivery row survives
// Erasure, and no Contact, Destination or provider reference must survive in the queued work.
type MessageUnsubscribed struct {
	WorkspaceID       int64     `json:"workspaceId"`
	OutboundMessageID int64     `json:"outboundMessageId"`
	ObservedAt        time.Time `json:"observedAt"`
}

func (*MessageUnsubscribed) EventName() string  { return NameMessageUnsubscribed }
func (*MessageUnsubscribed) EventVersion() int  { return 1 }
func (e *MessageUnsubscribed) Workspace() int64 { return e.WorkspaceID }
