package events

import "time"

const (
	NameTelegramStopObserved   = "telegram.stop_observed"
	NameTelegramResumeObserved = "telegram.resume_observed"
)

// TelegramStopObserved is an internal consent observation, consumed before the atomic stop.
// Message attribution is an immutable scalar snapshot, never a provider error or an ent row.
// ObservedAt is when the stop occurred, and Sequence the bot's update_id of an update (0 for a
// refused send or a stop recorded by hand), which orders two observations of one bot (ADR 0154).
type TelegramStopObserved struct {
	WorkspaceID       int64     `json:"workspaceId"`
	Destination       string    `json:"destination"`
	IntegrationID     int64     `json:"integrationId"`
	Sequence          int64     `json:"sequence,omitempty"`
	Via               string    `json:"via"`
	OutboundMessageID int64     `json:"outboundMessageId,omitempty"`
	ContactID         int64     `json:"contactId,omitempty"`
	BroadcastID       int64     `json:"broadcastId,omitempty"`
	AutomationID      int64     `json:"automationId,omitempty"`
	DedupID           string    `json:"dedupId,omitempty"`
	ObservedAt        time.Time `json:"observedAt,omitzero"`
}

func (*TelegramStopObserved) EventName() string            { return NameTelegramStopObserved }
func (*TelegramStopObserved) EventVersion() int            { return 1 }
func (e *TelegramStopObserved) Workspace() int64           { return e.WorkspaceID }
func (e *TelegramStopObserved) EventID() string            { return e.DedupID }
func (e *TelegramStopObserved) ObservationTime() time.Time { return e.ObservedAt }

// TelegramResumeObserved consumes a start or unblock independently of whether it lifts an opt-out.
// ObservedAt and Sequence are as a stop's.
type TelegramResumeObserved struct {
	WorkspaceID   int64     `json:"workspaceId"`
	Destination   string    `json:"destination"`
	IntegrationID int64     `json:"integrationId"`
	Sequence      int64     `json:"sequence,omitempty"`
	DedupID       string    `json:"dedupId,omitempty"`
	ObservedAt    time.Time `json:"observedAt,omitzero"`
}

func (*TelegramResumeObserved) EventName() string            { return NameTelegramResumeObserved }
func (*TelegramResumeObserved) EventVersion() int            { return 1 }
func (e *TelegramResumeObserved) Workspace() int64           { return e.WorkspaceID }
func (e *TelegramResumeObserved) EventID() string            { return e.DedupID }
func (e *TelegramResumeObserved) ObservationTime() time.Time { return e.ObservedAt }

func (*TelegramStopObserved) namesContact()   {}
func (*TelegramResumeObserved) namesContact() {}
