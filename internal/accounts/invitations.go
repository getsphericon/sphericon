package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

// InviteTokenTTL is how long an invitation link stays valid.
const InviteTokenTTL = 7 * 24 * time.Hour

// inviteTokenBytes of entropy encode to a 48-character URL-safe token.
const inviteTokenBytes = 36

var (
	// ErrEmailEmpty means the invited email is blank.
	ErrEmailEmpty = errors.New("accounts: invitation email is empty")
	// ErrAlreadyMember means the invited address already has a Membership in the Workspace.
	ErrAlreadyMember = errors.New("accounts: invited email is already a member")
	// ErrCredentialsRequired means the invited address has no Verified User, so the
	// invitee must give a name and a password to accept.
	ErrCredentialsRequired = errors.New("accounts: invitation needs a name and a password")
	// ErrSignInRequired means the invited address belongs to a Verified User and no
	// session was presented: only that User's own session may accept.
	ErrSignInRequired = errors.New("accounts: sign in to accept this invitation")
	// ErrWrongAccount means the presented session is another User's than the
	// Verified User the invitation is for.
	ErrWrongAccount = errors.New("accounts: this invitation is for another account")
	// ErrAlreadyVerified means the User's email was already verified: a link that
	// completes an Unverified User lost the race to another one.
	ErrAlreadyVerified = errors.New("accounts: the email is already verified")
)

// generateInviteToken returns a random, URL-safe invitation token: the raw value
// that travels in the invite link and is never stored.
func generateInviteToken() (string, error) {
	b := make([]byte, inviteTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashInviteToken returns the deterministic SHA-256 hex of a raw invite token. It
// is what the Invitation stores, so a presented token can be looked up by hash.
// SHA-256 (not bcrypt) is right here: the token is high-entropy random, so no slow
// hashing is needed and deterministic lookup is required.
func HashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
