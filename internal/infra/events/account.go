package events

// The events of a User's account. They belong to the Instance, not to a Workspace: a Sign-up, a
// password reset or a Passkey is the User's, whatever Workspaces they hold. None is Projected,
// so none has an Event row or reaches a Webhook endpoint; the consumers that ask for them (the
// Notices) read them from their jobs.
const (
	// NameUserVerificationRequested: the verification link of an Unverified User is asked for, by
	// a Sign-up or by a request to send it again (ADR 0030).
	NameUserVerificationRequested = "user.verification_requested"
	// NameUserPasswordResetRequested: a password reset link is asked for (forgot-password).
	NameUserPasswordResetRequested = "user.password_reset_requested"
	// NameUserEmailChangeRequested: a change of the login address is asked for; the link goes to
	// the new address.
	NameUserEmailChangeRequested = "user.email_change_requested"
	// NameUserDeletionRequested: the one-time deletion link of a User with no password is asked
	// for (ADR 0050).
	NameUserDeletionRequested = "user.deletion_requested"
	// NameUserAddressClaimed: a Sign-up or an email change asked for an address a Verified User
	// holds (ADR 0031).
	NameUserAddressClaimed = "user.address_claimed"
	// NameUserSignedUp: a Sign-up completed: the User is Verified with their first Workspace.
	NameUserSignedUp = "user.signed_up"
	// NameUserIdentityLinked: a Sign-in provider was linked to the account silently (ADR 0048).
	NameUserIdentityLinked = "user.identity_linked"
	// NameUserDeleted: a User was deleted (ADR 0050).
	NameUserDeleted = "user.deleted"
	// NameUserPasskeyAdded and NameUserPasskeyRemoved: a Passkey of the User was added or removed
	// (ADR 0052).
	NameUserPasskeyAdded   = "user.passkey_added"
	NameUserPasskeyRemoved = "user.passkey_removed"
)

// LinkRequested is a request for a one-time link to the address Email: a verification, a
// password reset, an email change (to the new address) or a User deletion, by Action. A request
// that must not reveal whether the address holds a User, or whether its mail budget is spent,
// publishes it all the same, with Deliver false: the request does the same work either way, and
// only a delivered one is mailed. UserID is the User the link is for, 0 when the address holds
// none. The link is minted when it is mailed, bound to the User as they are then, so the event
// carries no token.
type LinkRequested struct {
	Action string `json:"action"`
	UserID int64  `json:"userId,omitempty"`
	Email  string `json:"email"`
	// BrowserHash is the hash of the Sign-up browser a verification link trusts (ADR 0030), ""
	// for none and for the other links.
	BrowserHash string `json:"browserHash,omitempty"`
	Deliver     bool   `json:"deliver"`
}

func (e *LinkRequested) EventName() string { return e.Action }
func (*LinkRequested) EventVersion() int   { return 1 }

// AddressClaimed is a Sign-up or an email change that asked for the address Email, which a
// Verified User holds. Like LinkRequested, it is published whether or not the address's mail
// budget allows a mail, and Deliver says which.
type AddressClaimed struct {
	Email   string `json:"email"`
	Deliver bool   `json:"deliver"`
}

func (*AddressClaimed) EventName() string { return NameUserAddressClaimed }
func (*AddressClaimed) EventVersion() int { return 1 }

// UserSignedUp is a completed Sign-up: the User at Email is Verified with their first Workspace.
type UserSignedUp struct {
	UserID int64  `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

func (*UserSignedUp) EventName() string { return NameUserSignedUp }
func (*UserSignedUp) EventVersion() int { return 1 }

// IdentityLinked is the Sign-in provider Provider (its registry name) linked to the account of the
// User at Email without the User's own step; IdentityID is the linked identity, which the "this
// was not me" link undoes.
type IdentityLinked struct {
	UserID     int64  `json:"userId"`
	Email      string `json:"email"`
	Provider   string `json:"provider"`
	IdentityID int64  `json:"identityId"`
}

func (*IdentityLinked) EventName() string { return NameUserIdentityLinked }
func (*IdentityLinked) EventVersion() int { return 1 }

// UserDeleted is the deletion of the User who held the address Email.
type UserDeleted struct {
	UserID int64  `json:"userId"`
	Email  string `json:"email"`
}

func (*UserDeleted) EventName() string { return NameUserDeleted }
func (*UserDeleted) EventVersion() int { return 1 }

// PasskeyChange is a Passkey named Name added to or removed from the account of the User at
// Email, by Action.
type PasskeyChange struct {
	Action string `json:"action"`
	UserID int64  `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

func (e *PasskeyChange) EventName() string { return e.Action }
func (*PasskeyChange) EventVersion() int   { return 1 }
