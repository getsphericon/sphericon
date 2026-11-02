// Package holdreason names why a send is Held (GLOSSARY: Hold, ADR 0015): the one list of reasons
// Outbound send holds a send for, which a held Broadcast stores and the contract's HoldReason
// carries. It is a leaf, so the ent schema types its column with it. The holds package declares the
// Workspace-level reasons and Outbound send the source-level ones, each from this list.
package holdreason

// Reason is why a send is Held.
type Reason string

// The Workspace-level reasons: they describe the Workspace, not one send, and the layout's banner
// shows them.
const (
	WorkspaceSuspended       Reason = "workspace_suspended"
	WorkspacePendingDeletion Reason = "workspace_pending_deletion"
	StaffHold                Reason = "staff_hold"
)

// The source-level reasons: a member fixes them on what sends, its Integration or its Sending
// domain.
const (
	// NoIntegration: no Integration can send it; the one named is disabled or archived, or the
	// Workspace has no default.
	NoIntegration Reason = "no_integration"
	// UnverifiedDomain: an email's From is on no verified Sending domain of the Workspace.
	UnverifiedDomain Reason = "unverified_domain"
	// ProviderStopped: the provider has stopped the account behind the Integration (ADR 0067); it
	// lifts when a quota read finds it sending again.
	ProviderStopped Reason = "provider_stopped"
)

// all is every reason, Workspace-level first.
var all = []Reason{
	WorkspaceSuspended, WorkspacePendingDeletion, StaffHold,
	NoIntegration, UnverifiedDomain, ProviderStopped,
}

// Values is every reason, as an ent enum column takes them.
func (Reason) Values() []string {
	values := make([]string, len(all))
	for i, r := range all {
		values[i] = string(r)
	}
	return values
}

// Source is the source-level reasons.
func Source() []Reason {
	return []Reason{NoIntegration, UnverifiedDomain, ProviderStopped}
}
