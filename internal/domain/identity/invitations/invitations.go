// Package invitations owns the Invitation of an address to a Workspace (ADR 0102): issuing and
// reissuing it with its one-time link token, revoking it, the list of a Workspace's pending ones and
// the lookup of a pending one by its token. Accepting an Invitation makes or completes a User, so
// it is internal/domain/identity/users', which marks the Invitation accepted here.
//
// The module takes the raw client for the lookup by token: the token is the only handle, so no
// Workspace is known up front (ADR 0017). Everything else takes the Workspace's scope.
package invitations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// InviteTokenTTL is how long an invitation link stays valid.
const InviteTokenTTL = 7 * 24 * time.Hour

// inviteTokenBytes of entropy encode to a 48-character URL-safe token.
const inviteTokenBytes = 36

var (
	// ErrEmailEmpty means the invited email is blank.
	ErrEmailEmpty = validation.RefuseField("email", "invitations: invitation email is empty", i18n.ErrorsEmailEmpty)
	// ErrInvitationNotFound means no pending Invitation of that id is in the Workspace.
	ErrInvitationNotFound = validation.NotFound("invitations: invitation not found", i18n.ErrorsInvitationNotFound)
	// ErrInvitationLinkNotFound means an Invitation link's token names no pending, unexpired
	// Invitation.
	ErrInvitationLinkNotFound = validation.NotFound("invitations: invitation not found or expired", i18n.ErrorsInvitationLinkNotFound)
	// ErrAlreadyMember means the invited address already has a Membership in the Workspace.
	ErrAlreadyMember = validation.Conflict("invitations: invited email is already a member", i18n.ErrorsAlreadyMember)
)

// Module reads and writes Invitations.
type Module struct {
	client *ent.Client
	bus    *events.Bus
}

// New builds the module over the ent client; the bus opens the transactions of an issue and a
// revocation, which record their Audit entries.
func New(client *ent.Client, bus *events.Bus) *Module {
	return &Module{client: client, bus: bus}
}

// PendingInvitations lists the unaccepted Invitations of the Workspace s with their inviters,
// oldest first.
func PendingInvitations(ctx context.Context, s *ent.Scoped) ([]*ent.Invitation, error) {
	return s.Invitation().Query().
		Where(invitation.AcceptedAtIsNil()).
		WithInviter().
		Order(ent.Asc(invitation.FieldID)).
		All(ctx)
}

// PendingInvitation looks up a pending, unexpired Invitation by its raw token, with
// its Workspace edge. The token is the authorization, so no Workspace is known up
// front. ErrInvitationLinkNotFound when there is none.
func (m *Module) PendingInvitation(ctx context.Context, token string) (*ent.Invitation, error) {
	inv, err := m.client.Invitation.Query().
		Where(invitation.TokenHash(HashInviteToken(token)), PendingAt(time.Now())).
		WithWorkspace().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrInvitationLinkNotFound
	}
	return inv, err
}

// PendingAt is the one definition of an Invitation still pending at now: not accepted and not
// expired.
func PendingAt(now time.Time) predicate.Invitation {
	return invitation.And(invitation.AcceptedAtIsNil(), invitation.ExpiresAtGT(now))
}

// MarkAcceptedIn marks the Invitation id of the scoped Workspace accepted, on the transaction of
// the acceptance that made its Membership.
func MarkAcceptedIn(ctx context.Context, ts *ent.Scoped, id int64) error {
	return ts.Invitation().UpdateOneID(id).SetAcceptedAt(time.Now()).Exec(ctx)
}

// InviteInput is a new (or reissued) Invitation request. The token, its hash and
// the expiry are the Invite operation's to decide, not the caller's.
type InviteInput struct {
	Email     string
	Role      invitation.Role
	InvitedBy int64
}

// Invite creates the Invitation for an email, or reissues the pending one (token,
// expiry, role; a prior acceptance is cleared), and records `invitation.create` in
// the same transaction. It generates the one-time token, stores only its hash, sets
// the expiry InviteTokenTTL ahead and returns the raw token for the caller's link;
// the token itself is never logged. ErrEmailEmpty for a blank email, ErrAlreadyMember
// when the address already has a Membership in the Workspace. Delivery is the
// caller's concern.
func (m *Module) Invite(ctx context.Context, s *ent.Scoped, actor events.Actor, in InviteInput) (*ent.Invitation, string, error) {
	email := throttle.NormalizeEmail(in.Email)
	if email == "" {
		return nil, "", ErrEmailEmpty
	}
	token, err := generateInviteToken()
	if err != nil {
		return nil, "", err
	}
	tokenHash := HashInviteToken(token)
	expiresAt := time.Now().Add(InviteTokenTTL)

	var inv *ent.Invitation
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		member, err := ts.Membership().Query().
			Where(membership.HasUserWith(user.Email(email))).
			Exist(ctx)
		if err != nil {
			return err
		}
		if member {
			return ErrAlreadyMember
		}
		existing, err := ts.Invitation().Query().Where(invitation.Email(email)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			inv, err = ts.Invitation().Create().
				SetEmail(email).SetRole(in.Role).SetTokenHash(tokenHash).
				SetExpiresAt(expiresAt).SetInvitedBy(in.InvitedBy).
				Save(ctx)
		case err != nil:
		default:
			inv, err = ts.Invitation().UpdateOneID(existing.ID).
				SetRole(in.Role).SetTokenHash(tokenHash).
				SetExpiresAt(expiresAt).SetInvitedBy(in.InvitedBy).
				ClearAcceptedAt().
				Save(ctx)
		}
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionInvitationCreate,
			TargetType:  invitation.Label,
			TargetID:    strconv.FormatInt(inv.ID, 10),
			TargetName:  inv.Email,
			Diff:        map[string]any{invitation.FieldRole: events.Became(string(inv.Role))},
		})
	})
	if err != nil {
		return nil, "", err
	}
	return inv, token, nil
}

// RevokeInvitation deletes a pending Invitation and records `invitation.revoke`.
// ErrInvitationNotFound when the Invitation is not in the scoped Workspace.
func (m *Module) RevokeInvitation(ctx context.Context, s *ent.Scoped, actor events.Actor, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		inv, err := ts.Invitation().Query().Where(invitation.ID(id)).Only(ctx)
		if ent.IsNotFound(err) {
			return ErrInvitationNotFound
		}
		if err != nil {
			return err
		}
		if err := ts.Invitation().DeleteOneID(id).Exec(ctx); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionInvitationRevoke,
			TargetType:  invitation.Label,
			TargetID:    strconv.FormatInt(id, 10),
			TargetName:  inv.Email,
		})
	})
}

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
