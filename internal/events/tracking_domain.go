package events

// NameTrackingDomainChanged is the bus type of a Tracking domain state change (ADR 0038, 0071):
// pending, dns_verified or serving became another of them.
const NameTrackingDomainChanged = "tracking_domain.changed"

// TrackingDomainChanged is published in the transaction that stores the new state, by a
// check of the host. It is not a data-plane fact, so it is Unprojected: no Event
// row, no automation trigger, no webhook.
type TrackingDomainChanged struct {
	WorkspaceID      int64  `json:"workspaceId"`
	TrackingDomainID int64  `json:"trackingDomainId"`
	Host             string `json:"host"`
	From             string `json:"from"`
	To               string `json:"to"`
}

func (*TrackingDomainChanged) EventName() string  { return NameTrackingDomainChanged }
func (*TrackingDomainChanged) EventVersion() int  { return 1 }
func (e *TrackingDomainChanged) Workspace() int64 { return e.WorkspaceID }
func (*TrackingDomainChanged) Unprojected()       {}

// Project is never persisted (the event is Unprojected); it only names the action.
func (*TrackingDomainChanged) Project() Projection {
	return Projection{Action: NameTrackingDomainChanged}
}
