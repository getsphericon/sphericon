package apitokens

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrConnectionNotFound refuses a Connection that is not in the Workspace, is already revoked or
// is another User's while the caller may not see it.
var ErrConnectionNotFound = validation.NotFound("apitokens: connection not found", i18n.ErrorsConnectionNotFound)

// Connections lists the active Connections of the Workspace s that caller (the User's Membership
// of it) may see, oldest first, each with its Membership and that Membership's User: the User's
// own, or every member's when the Role holds members:manage (ADR 0061). That rule is the Policy of
// a Connection, so every surface that lists them gets it.
func Connections(ctx context.Context, s *ent.Scoped, caller *ent.Membership) ([]*ent.ApiToken, error) {
	return s.ApiToken().Query().
		Where(isConnection(), visibleTo(caller), apitoken.RevokedAtIsNil()).
		WithMembership(func(q *ent.MembershipQuery) { q.WithUser() }).
		Order(ent.Asc(apitoken.FieldID)).
		All(ctx)
}

// RevokeConnection retires the active Connection id of the Workspace s on behalf of caller, under
// the Policy of Connections: the caller's own, or anyone's when the Role holds members:manage. A
// Connection the caller may not see is ErrConnectionNotFound, as an unknown one is.
func RevokeConnection(ctx context.Context, s *ent.Scoped, caller *ent.Membership, id int64, at time.Time) error {
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(id), isConnection(), visibleTo(caller), apitoken.RevokedAtIsNil()).
		SetRevokedAt(at).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

// Grant is what an OAuth consent granted a connector (ADR 0016, ADR 0061): the Connection that
// IssueConnection makes of it is the API token of the approving User's Membership, named after
// the client, carrying exactly the consented scopes and expiring at ExpiresAt.
type Grant struct {
	ClientName    string
	Scopes        []string
	MembershipID  int64
	OAuthClientID int64
	ExpiresAt     time.Time
}

// IssueConnection stores the Connection of the grant g in the Workspace s and returns it with
// its secret. The Membership is its own, so it ends with it, and it names the client it was
// approved for, which tells it apart from a Personal token. The scopes are the consent's and
// are not checked again here: only the exchange of an authorization code calls it
// (internal/usecase/connectionissuance), after it has checked the code and its PKCE verifier.
func IssueConnection(ctx context.Context, s *ent.Scoped, g Grant) (Minted, error) {
	prefix, err := credentials.GenerateTokenPrefix()
	if err != nil {
		return Minted{}, err
	}
	secret, err := credentials.GenerateTokenSecret()
	if err != nil {
		return Minted{}, err
	}
	token, err := s.ApiToken().Create().
		SetName(g.ClientName).
		SetPrefix(prefix).
		SetSecretHash(credentials.HashSecret(secret)).
		SetScopes(g.Scopes).
		SetMembershipID(g.MembershipID).
		SetOauthClientID(g.OAuthClientID).
		SetExpiresAt(g.ExpiresAt).
		Save(ctx)
	if err != nil {
		return Minted{}, err
	}
	return Minted{Token: token, Value: credentials.TokenValue(prefix, secret)}, nil
}

// RotateConnection gives the active Connection token of the Workspace s a fresh secret, scopes
// and expiry, keeping its row and prefix, and returns its new value; false when it was revoked
// meanwhile. Only the exchange of a refresh token calls it (internal/usecase/connectionissuance),
// after it has checked and claimed the refresh token.
func RotateConnection(ctx context.Context, s *ent.Scoped, token *ent.ApiToken, scopes []string, expiresAt time.Time) (value string, rotated bool, err error) {
	secret, err := credentials.GenerateTokenSecret()
	if err != nil {
		return "", false, err
	}
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(token.ID), apitoken.RevokedAtIsNil()).
		SetSecretHash(credentials.HashSecret(secret)).
		SetScopes(scopes).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil || n == 0 {
		return "", false, err
	}
	return credentials.TokenValue(token.Prefix, secret), true, nil
}

// RevokeGrant revokes the Connection id of the Workspace s unless it is revoked already: a
// rotated refresh token of its grant was presented again, so the connector's credentials may be
// stolen (ADR 0016).
func RevokeGrant(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	_, err := s.ApiToken().Update().
		Where(apitoken.ID(id), apitoken.RevokedAtIsNil()).
		SetRevokedAt(at).
		Save(ctx)
	return err
}
