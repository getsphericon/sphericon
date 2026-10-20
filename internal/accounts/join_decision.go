package accounts

// JoinFacts is everything the domain-join decision rests on, read before it is made (ADR
// 0051). The decision is a pure function of them, so every branch is tested on its inputs
// alone, and what the join writes is applyDomainJoin's.
type JoinFacts struct {
	// ProvenNow: the address became a Proven email in this very step, stamped from empty. An
	// address proven before, or one that was merely recorded as verified, never joins.
	ProvenNow bool
	// DomainHeld: a Workspace has a verified Join domain equal to the address's domain
	// (not a parent of it: subdomains do not match). The lookup reads the stored
	// verification, never DNS.
	DomainHeld bool
	// SwitchOn: that Workspace's domain join is switched on.
	SwitchOn bool
	// Suspended: that Workspace is suspended.
	Suspended bool
	// InDeletion: that Workspace is in its deletion window.
	InDeletion bool
	// HasMembership: the User already has a Membership in that Workspace, of any Role.
	HasMembership bool
	// SignupPermits is whether the sign-up gate lets this person in (ADR 0042): the User
	// already exists, or sign-up is open. A domain match never opens a closed Instance.
	SignupPermits bool
}

// JoinOutcome is what the proof of an address does about domain join.
type JoinOutcome int

const (
	// JoinNothing: no Membership is created; the Reason says why.
	JoinNothing JoinOutcome = iota
	// JoinAsMember: the User becomes a `member` of the Workspace, source `domain`.
	JoinAsMember
)

// JoinReason is why nothing was joined.
type JoinReason string

const (
	JoinReasonNotProvenNow    JoinReason = "not_proven_now"
	JoinReasonNoVerifiedHold  JoinReason = "no_verified_domain"
	JoinReasonSwitchOff       JoinReason = "switch_off"
	JoinReasonWorkspaceFrozen JoinReason = "workspace_frozen"
	JoinReasonAlreadyMember   JoinReason = "already_member"
	JoinReasonSignupClosed    JoinReason = "signup_closed"
)

// JoinDecision is an outcome with the Reason of a refusal.
type JoinDecision struct {
	Outcome JoinOutcome
	Reason  JoinReason
}

// DecideJoin is the one domain-join decision. A User joins only at the moment an address is
// proven (never because the switch was turned on later), only the Workspace whose verified
// Join domain equals the address's domain, only while its switch is on and it is neither
// suspended nor in its deletion window, only without a Membership already, and only where
// the sign-up gate lets the person in.
func DecideJoin(f JoinFacts) JoinDecision {
	switch {
	case !f.ProvenNow:
		return JoinDecision{Reason: JoinReasonNotProvenNow}
	case !f.DomainHeld:
		return JoinDecision{Reason: JoinReasonNoVerifiedHold}
	case !f.SwitchOn:
		return JoinDecision{Reason: JoinReasonSwitchOff}
	case f.Suspended || f.InDeletion:
		return JoinDecision{Reason: JoinReasonWorkspaceFrozen}
	case f.HasMembership:
		return JoinDecision{Reason: JoinReasonAlreadyMember}
	case !f.SignupPermits:
		return JoinDecision{Reason: JoinReasonSignupClosed}
	default:
		return JoinDecision{Outcome: JoinAsMember}
	}
}
