package events

import "time"

const (
	NameTelegramStopObserved   = "telegram.stop_observed"
	NameTelegramResumeObserved = "telegram.resume_observed"
)

// TelegramStopObserved is an internal consent observation, consumed before the atomic stop.
// Message attribution is an immutable scalar snapshot, never a provider error or an ent row.
type TelegramStopObserved struct {
	WorkspaceID       int64     `json:"workspaceId"`
	Destination       string    `json:"destination"`
	IntegrationID     int64     `json:"integrationId"`
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
type TelegramResumeObserved struct {
	WorkspaceID   int64     `json:"workspaceId"`
	Destination   string    `json:"destination"`
	IntegrationID int64     `json:"integrationId"`
	DedupID       string    `json:"dedupId,omitempty"`
	ObservedAt    time.Time `json:"observedAt,omitzero"`
}

func (*TelegramResumeObserved) EventName() string            { return NameTelegramResumeObserved }
func (*TelegramResumeObserved) EventVersion() int            { return 1 }
func (e *TelegramResumeObserved) Workspace() int64           { return e.WorkspaceID }
func (e *TelegramResumeObserved) EventID() string            { return e.DedupID }
func (e *TelegramResumeObserved) ObservationTime() time.Time { return e.ObservedAt }
