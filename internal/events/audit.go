package events

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/getsphericon/sphericon/internal/logging"
)

// NameAuditEntry is the bus type of an Audit entry (ADR 0022). Core only carries the
// type and the publish seam; the table, the subscriber and the read surfaces live in
// ee/audit, gated by the license key. Without a license the event is published and
// nobody consumes it.
const NameAuditEntry = "audit.entry"

// Actor kinds of an Audit entry. Scopes built from a secret rather than a login carry
// ActorIngest, which the seam never records. The kinds and the Actor type live in the
// generated scoped client (package ent), which carries the actor of a scope.
const (
	ActorUser     = ent.ActorUser
	ActorAPIToken = ent.ActorAPIToken
	ActorStaff    = ent.ActorStaff
	ActorService  = ent.ActorService
	ActorSystem   = ent.ActorSystem
	ActorIngest   = ent.ActorIngest
)

// IsStaff reports whether an actor kind is the platform's own staff identity, a human
// Staff member or a Service credential: the kinds no customer-facing read may identify.
func IsStaff(kind string) bool { return kind == ActorStaff || kind == ActorService }

// StaffLabel is how a platform Staff member appears to the customer on every read
// surface (ADR 0022): the staff identity is never exposed.
const StaffLabel = "Sphericon staff"

// Actor is who performed a change. Name is a display snapshot (a User's name) so the
// entry stays readable after the actor is gone.
type Actor = ent.Actor

// Actions recorded through the explicit seam (ADR 0022): raw-client packages and
// actions that are not a row write. A test drives every one of them and compares with
// ExplicitAuditActions, so adding an explicit path means listing it here.
const (
	ActionMembershipUpdate = "membership.update"
	ActionInvitationCreate = "invitation.create"
	ActionInvitationAccept = "invitation.accept"
	ActionInvitationRevoke = "invitation.revoke"
	ActionUserLogin        = "user.login"
	// The User proved its email by the emailed link and set its password (ADR 0030).
	ActionUserEmailVerify    = "user.email_verify"
	ActionUserPasswordChange = "user.password_change"
	// "Sign out everywhere": every session of the User ends (ADR 0020).
	ActionUserSignOutEverywhere = "user.sign_out_everywhere"
	// The Second factor (ADR 0020).
	ActionUserSecondFactorEnroll      = "user.second_factor_enroll"
	ActionUserSecondFactorDisable     = "user.second_factor_disable"
	ActionUserSecondFactorReset       = "user.second_factor_reset"
	ActionUserRecoveryCodesRegenerate = "user.recovery_codes_regenerate"
	ActionUserRecoveryCodeUse         = "user.recovery_code_use"
	ActionWorkspaceUpdate             = "workspace.update"
	ActionWorkspaceSuspend            = "workspace.suspend"
	ActionWorkspaceUnsuspend          = "workspace.unsuspend"
	ActionWorkspaceAbuseExempt        = "workspace.abuse_exempt"
	ActionWorkspaceAbuseUnexempt      = "workspace.abuse_unexempt"
	ActionContactImport               = "contact.import"
	ActionAuditLogExport              = "audit_log.export"
	// A sample event was sent to a Webhook endpoint (ADR 0039): no row changes.
	ActionWebhookEndpointTest = "webhook_endpoint.test"
)

// ExplicitAuditActions lists every action emitted by an explicit RecordAudit call.
var ExplicitAuditActions = []string{
	ActionMembershipUpdate,
	ActionInvitationCreate,
	ActionInvitationAccept,
	ActionInvitationRevoke,
	ActionUserLogin,
	ActionUserEmailVerify,
	ActionUserPasswordChange,
	ActionUserSignOutEverywhere,
	ActionUserSecondFactorEnroll,
	ActionUserSecondFactorDisable,
	ActionUserSecondFactorReset,
	ActionUserRecoveryCodesRegenerate,
	ActionUserRecoveryCodeUse,
	ActionWorkspaceUpdate,
	ActionWorkspaceSuspend,
	ActionWorkspaceUnsuspend,
	ActionWorkspaceAbuseExempt,
	ActionWorkspaceAbuseUnexempt,
	ActionContactImport,
	ActionAuditLogExport,
	ActionWebhookEndpointTest,
}

// Unprojected is an optional DomainEvent capability: an event that is not a
// data-plane fact. The persist consumer (no Event row), the automation trigger
// consumer and the webhooks consumer skip it, and an endpoint with an empty
// subscription list never matches it (see IsUnprojected).
type Unprojected interface {
	Unprojected()
}

// IsUnprojected reports whether the bus event type opts out of the Event projection.
func IsUnprojected(name string) bool {
	newEvent, ok := registry[name]
	if !ok {
		return false
	}
	_, unprojected := newEvent().(Unprojected)
	return unprojected
}

// AuditEntry is emitted in the transaction of the change it records; the envelope id
// is the dedupe key, so at-least-once redelivery writes one row.
type AuditEntry struct {
	WorkspaceID int64          `json:"workspaceId"`
	Actor       Actor          `json:"actor"`
	Action      string         `json:"action"` // <entity>.<verb>
	TargetType  string         `json:"targetType"`
	TargetID    string         `json:"targetId,omitempty"`
	TargetName  string         `json:"targetName,omitempty"`
	Diff        map[string]any `json:"diff,omitempty"`
	RequestID   string         `json:"requestId,omitempty"`
	IP          string         `json:"ip,omitempty"`
	UserAgent   string         `json:"userAgent,omitempty"`
}

func (*AuditEntry) EventName() string  { return NameAuditEntry }
func (*AuditEntry) EventVersion() int  { return 1 }
func (e *AuditEntry) Workspace() int64 { return e.WorkspaceID }
func (*AuditEntry) Unprojected()       {}

// MaskStaff replaces a staff actor (a Staff member or a Service credential) with the
// customer-facing label and drops the staff id, for every surface that leaves the
// platform (ADR 0022). The stored entry keeps the real identity.
func (e *AuditEntry) MaskStaff() {
	if IsStaff(e.Actor.Kind) {
		e.Actor = Actor{Kind: ActorStaff, Name: StaffLabel}
	}
}

// Project is never persisted (AuditEntry is Unprojected); it only names the action.
func (e *AuditEntry) Project() Projection { return Projection{Action: e.Action} }

// RecordAudit publishes an Audit entry through pub, inside the caller's transaction
// (the seam, ADR 0022). A missing or ingest actor is skipped: ingest traffic never
// reaches the log. The request id, address and user agent come from ctx.
func RecordAudit(ctx context.Context, pub Publisher, entry *AuditEntry) error {
	if entry.Actor.Kind == "" || entry.Actor.Kind == ActorIngest {
		return nil
	}
	entry.RequestID, _ = logging.RequestIDFromContext(ctx)
	entry.IP = clientip.FromContext(ctx)
	entry.UserAgent = clientip.UserAgentFromContext(ctx)
	return pub.Publish(ctx, entry)
}

// AuditFilter narrows the Audit log. Every field is optional and they combine with AND;
// the zero value matches every entry. The site list and the CSV export take the same
// filter, so an export matches what the page shows.
type AuditFilter struct {
	// From is inclusive and To exclusive; a zero time is unbounded.
	From, To   time.Time
	ActorKind  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	IP         string
	RequestID  string
}
