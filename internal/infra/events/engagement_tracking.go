package events

import (
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// EngagementTracking is an observation of a Destination's Engagement tracking consent (ADR 0120)
// that set the consent: the evidentiary System event its derived row is projected from, never
// deleted by age (ADR 0019). Channel is the Destination's (email, sms, telegram), and the action
// is the Channel's own (EngagementTrackingGranted, EngagementTrackingDenied). Destination is the
// email address on email and the number in E.164 on sms, which the Event row holds as its email or
// its phone; ContactID is the Contact holding the Destination, 0 when none does. Provenance is who
// recorded it (page, manual, imported, onsite_form), OccurredAt when the observation occurred.
type EngagementTracking struct {
	WorkspaceID int64            `json:"workspaceId"`
	ContactID   int64            `json:"contactId,omitempty"`
	Channel     channels.Channel `json:"channel"`
	Destination string           `json:"destination"`
	Provenance  string           `json:"provenance"`
	OccurredAt  time.Time        `json:"occurredAt"`
}

// project is the Event row of e under action: the Destination as the row's email or phone, the
// provenance as its property, dated when the observation occurred.
func (e *EngagementTracking) project(action string) Projection {
	p := Projection{
		Action: action, ContactID: e.ContactID, OccurredAt: e.OccurredAt,
		Properties: map[string]any{"provenance": e.Provenance},
	}
	switch e.Channel {
	case channels.Email:
		p.Subject, p.Email = e.Destination, e.Destination
	case channels.SMS:
		p.Phone = e.Destination
	case channels.Telegram:
		// A Telegram Destination is a user id, which the Event row has no column for: the Event
		// names it by its Contact.
	}
	return p
}

// EngagementTrackingGranted is a Destination agreeing to have its opens and clicks tracked
// (`<channel>.engagement_tracking_granted`). Like a Confirmation's proof, Erasure deletes it: it is
// consent of a person who is gone, and losing it falls back to the default.
type EngagementTrackingGranted struct {
	EngagementTracking
}

// EngagementTrackingDenied is a Destination refusing to have its opens and clicks tracked
// (`<channel>.engagement_tracking_denied`). Like an unsubscribe, Erasure keeps it as an anonymous
// row, as it keeps the refusal.
type EngagementTrackingDenied struct {
	EngagementTracking
}

// grantedActions and deniedActions are the actions of a Channel's Engagement tracking Events.
var (
	grantedActions = map[channels.Channel]string{
		channels.Email:    systemevent.EmailEngagementTrackingGranted,
		channels.SMS:      systemevent.SmsEngagementTrackingGranted,
		channels.Telegram: systemevent.TelegramEngagementTrackingGranted,
	}
	deniedActions = map[channels.Channel]string{
		channels.Email:    systemevent.EmailEngagementTrackingDenied,
		channels.SMS:      systemevent.SmsEngagementTrackingDenied,
		channels.Telegram: systemevent.TelegramEngagementTrackingDenied,
	}
)

func (e *EngagementTrackingGranted) EventName() string          { return grantedActions[e.Channel] }
func (*EngagementTrackingGranted) EventVersion() int            { return 1 }
func (e *EngagementTrackingGranted) Workspace() int64           { return e.WorkspaceID }
func (e *EngagementTrackingGranted) ObservationTime() time.Time { return e.OccurredAt }
func (e *EngagementTrackingGranted) Project() Projection        { return e.project(e.EventName()) }

func (e *EngagementTrackingDenied) EventName() string          { return deniedActions[e.Channel] }
func (*EngagementTrackingDenied) EventVersion() int            { return 1 }
func (e *EngagementTrackingDenied) Workspace() int64           { return e.WorkspaceID }
func (e *EngagementTrackingDenied) ObservationTime() time.Time { return e.OccurredAt }
func (e *EngagementTrackingDenied) Project() Projection        { return e.project(e.EventName()) }
