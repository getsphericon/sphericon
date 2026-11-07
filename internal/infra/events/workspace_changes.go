package events

import "time"

// The changes of a Workspace's members, standing and configuration that its people are told of.
// Each belongs to its Workspace, and none is Projected: no Event row, no automation trigger, no
// webhook. Each is published in the transaction of the change it reports, so it exists iff the
// change commits.
const (
	// NameInvitationCreated: an Invitation to the Workspace was made or renewed.
	NameInvitationCreated = "invitation.created"
	// NameWorkspaceSecondFactorRequired: the Workspace's Two-factor requirement was switched on
	// (ADR 0020).
	NameWorkspaceSecondFactorRequired = "workspace.second_factor_required"
	// NameMembershipSecondFactorReminderDue: the grace of a Member without a Second factor ends
	// soon, and the reminder of that grace was claimed (ADR 0020).
	NameMembershipSecondFactorReminderDue = "membership.second_factor_reminder_due"
	// NameWorkspaceSuspended: the Workspace was suspended (ADR 0007).
	NameWorkspaceSuspended = "workspace.suspended"
	// NameWorkspaceAbuseWarned: the abuse detector warned the Workspace of a breach (ADR 0026).
	NameWorkspaceAbuseWarned = "workspace.abuse_warned"
	// NameJoinDomainLost: a verified Join domain of the Workspace lost its verification, by a
	// re-check or by a hand-over to another Workspace (ADR 0051).
	NameJoinDomainLost = "join_domain.lost"
	// NameSendingDomainVerificationLost: a check found that a verified Sending domain lost its
	// DKIM verification (ADR 0046).
	NameSendingDomainVerificationLost = "sending_domain.verification_lost"
	// NameTrackingDomainReplaced: a replacement Tracking host took over from the current one
	// (ADR 0071).
	NameTrackingDomainReplaced = "tracking_domain.replaced"
	// NameTrackingDomainRemoved: a retired Tracking host no longer reaches the edge and was
	// removed (ADR 0071).
	NameTrackingDomainRemoved = "tracking_domain.removed"
	// NameWebhookEndpointDisabled: the delivery turned a Webhook endpoint off (ADR 0039).
	NameWebhookEndpointDisabled = "webhook_endpoint.disabled"
)

// InvitationCreated is an Invitation of Email to the Workspace by the User InvitedBy (0 for
// none), with the one-time Token of its link: the token is stored hashed, so the event is the
// one place it is kept until it is mailed.
type InvitationCreated struct {
	WorkspaceID  int64  `json:"workspaceId"`
	InvitationID int64  `json:"invitationId"`
	Email        string `json:"email"`
	Token        string `json:"token"`
	InvitedBy    int64  `json:"invitedBy,omitempty"`
}

func (*InvitationCreated) EventName() string  { return NameInvitationCreated }
func (*InvitationCreated) EventVersion() int  { return 1 }
func (e *InvitationCreated) Workspace() int64 { return e.WorkspaceID }

// SecondFactorRequired is the Workspace's Two-factor requirement switched on. Who has no Second
// factor is read when it is handled, so a requirement switched off again in between asks no one.
type SecondFactorRequired struct {
	WorkspaceID int64 `json:"workspaceId"`
}

func (*SecondFactorRequired) EventName() string  { return NameWorkspaceSecondFactorRequired }
func (*SecondFactorRequired) EventVersion() int  { return 1 }
func (e *SecondFactorRequired) Workspace() int64 { return e.WorkspaceID }

// SecondFactorReminderDue is the claimed reminder of the Membership MembershipID, whose grace
// under the Workspace's Two-factor requirement ends at Deadline.
type SecondFactorReminderDue struct {
	WorkspaceID  int64     `json:"workspaceId"`
	MembershipID int64     `json:"membershipId"`
	Deadline     time.Time `json:"deadline"`
}

func (*SecondFactorReminderDue) EventName() string  { return NameMembershipSecondFactorReminderDue }
func (*SecondFactorReminderDue) EventVersion() int  { return 1 }
func (e *SecondFactorReminderDue) Workspace() int64 { return e.WorkspaceID }

// WorkspaceSuspended is the Workspace suspended. Why and by whom is on the Workspace, read when
// it is handled, so a suspension lifted in between tells no one.
type WorkspaceSuspended struct {
	WorkspaceID int64 `json:"workspaceId"`
}

func (*WorkspaceSuspended) EventName() string  { return NameWorkspaceSuspended }
func (*WorkspaceSuspended) EventVersion() int  { return 1 }
func (e *WorkspaceSuspended) Workspace() int64 { return e.WorkspaceID }

// AbuseWarned is the abuse detector's warning of a breach on the Workspace's sending domain
// Domain, worded by the detector as it found it: the breached rate's name (Metric), its value and
// its limit as percentages, and the counts behind the rate.
type AbuseWarned struct {
	WorkspaceID int64  `json:"workspaceId"`
	Domain      string `json:"domain"`
	Metric      string `json:"metric"`
	Rate        string `json:"rate"`
	Limit       string `json:"limit"`
	Count       int64  `json:"count"`
	Total       int64  `json:"total"`
}

func (*AbuseWarned) EventName() string  { return NameWorkspaceAbuseWarned }
func (*AbuseWarned) EventVersion() int  { return 1 }
func (e *AbuseWarned) Workspace() int64 { return e.WorkspaceID }

// JoinDomainLost is the Join domain Domain of the Workspace no longer verified; Record is the
// DNS record that proves it again.
type JoinDomainLost struct {
	WorkspaceID  int64  `json:"workspaceId"`
	JoinDomainID int64  `json:"joinDomainId"`
	Domain       string `json:"domain"`
	Record       string `json:"record"`
}

func (*JoinDomainLost) EventName() string  { return NameJoinDomainLost }
func (*JoinDomainLost) EventVersion() int  { return 1 }
func (e *JoinDomainLost) Workspace() int64 { return e.WorkspaceID }

// SendingDomainVerificationLost is the Sending domain Domain of the Workspace no longer
// verified; Record is the DKIM record that verifies it again.
type SendingDomainVerificationLost struct {
	WorkspaceID     int64  `json:"workspaceId"`
	SendingDomainID int64  `json:"sendingDomainId"`
	Domain          string `json:"domain"`
	Record          string `json:"record"`
}

func (*SendingDomainVerificationLost) EventName() string  { return NameSendingDomainVerificationLost }
func (*SendingDomainVerificationLost) EventVersion() int  { return 1 }
func (e *SendingDomainVerificationLost) Workspace() int64 { return e.WorkspaceID }

// TrackingDomainReplaced is the Tracking host Host serving in place of Old, which is retired.
type TrackingDomainReplaced struct {
	WorkspaceID      int64  `json:"workspaceId"`
	TrackingDomainID int64  `json:"trackingDomainId"`
	Host             string `json:"host"`
	Old              string `json:"old"`
}

func (*TrackingDomainReplaced) EventName() string  { return NameTrackingDomainReplaced }
func (*TrackingDomainReplaced) EventVersion() int  { return 1 }
func (e *TrackingDomainReplaced) Workspace() int64 { return e.WorkspaceID }

// TrackingDomainRemoved is the retired Tracking host Host removed once its CNAME no longer reaches
// the edge.
type TrackingDomainRemoved struct {
	WorkspaceID      int64  `json:"workspaceId"`
	TrackingDomainID int64  `json:"trackingDomainId"`
	Host             string `json:"host"`
}

func (*TrackingDomainRemoved) EventName() string  { return NameTrackingDomainRemoved }
func (*TrackingDomainRemoved) EventVersion() int  { return 1 }
func (e *TrackingDomainRemoved) Workspace() int64 { return e.WorkspaceID }

// WebhookEndpointDisabled is the Webhook endpoint EndpointID turned off by the delivery, for
// Reason (the endpoint's disabled reason). Origin is the scheme and host of its URL: the path and
// the query may carry a secret, so they are never carried.
type WebhookEndpointDisabled struct {
	WorkspaceID int64  `json:"workspaceId"`
	EndpointID  int64  `json:"endpointId"`
	Origin      string `json:"origin"`
	Reason      string `json:"reason"`
}

func (*WebhookEndpointDisabled) EventName() string  { return NameWebhookEndpointDisabled }
func (*WebhookEndpointDisabled) EventVersion() int  { return 1 }
func (e *WebhookEndpointDisabled) Workspace() int64 { return e.WorkspaceID }
