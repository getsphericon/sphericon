package apitokens

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/membership"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrPersonalTokenNotFound refuses a Personal token that is not in the Workspace, is already
// revoked or is another User's while the caller may not see it.
var ErrPersonalTokenNotFound = validation.NotFound("apitokens: personal token not found", "errors.personal_token_not_found")

// PersonalInput is what a User chooses when minting a Personal token. Its scopes are the
// contract's PersonalScope, the scopes a User may give a credential of their own (ADR 0061): a
// token-only scope such as tokens:write, which would let the token outlive its User, is none, and
// a request that names one is not decoded.
type PersonalInput struct {
	Name      string
	Scopes    []siteapi.PersonalScope
	ExpiresAt *time.Time
}

// GrantableScopes is the scopes a User whose Role is role may give a Personal token, in the
// contract's order: each PersonalScope accounts.MayGrant allows the Role (ADR 0061).
func GrantableScopes(role membership.Role) []siteapi.PersonalScope {
	return lo.Filter(siteapi.PersonalScope("").AllValues(), func(scope siteapi.PersonalScope, _ int) bool {
		return accounts.MayGrant(role, []string{string(scope)}) == nil
	})
}

// MintPersonal mints a Personal token of caller, the User's Membership of the Workspace s: a
// Connection without the application (ADR 0061). Its scopes are bounded by accounts.MayGrant, as a
// Connection's are on the consent screen: the first one beyond caller's Role refuses the whole
// token. It is returned with its Membership and that Membership's User, and its secret, shown once.
func MintPersonal(ctx context.Context, s *ent.Scoped, caller *ent.Membership, in PersonalInput) (Minted, error) {
	requested := lo.Map(in.Scopes, func(scope siteapi.PersonalScope, _ int) string { return string(scope) })
	name, scopes, err := checked(Input{Name: in.Name, Scopes: requested})
	if err != nil {
		return Minted{}, err
	}
	if err := accounts.MayGrant(caller.Role, scopes); err != nil {
		return Minted{}, err
	}
	minted, err := mint(ctx, s, name, scopes, in.ExpiresAt, &caller.ID)
	if err != nil {
		return Minted{}, err
	}
	minted.Token, err = s.ApiToken().Query().
		Where(apitoken.ID(minted.Token.ID)).
		WithMembership(func(q *ent.MembershipQuery) { q.WithUser() }).
		Only(ctx)
	if err != nil {
		return Minted{}, err
	}
	return minted, nil
}

// PersonalTokens lists the active Personal tokens of the Workspace s that caller may see, oldest
// first, each with its Membership and that Membership's User: the caller's own, or every member's
// when the Role holds members:manage, the Policy of a Connection (ADR 0061).
func PersonalTokens(ctx context.Context, s *ent.Scoped, caller *ent.Membership) ([]*ent.ApiToken, error) {
	return s.ApiToken().Query().
		Where(isPersonalToken(), visibleTo(caller), apitoken.RevokedAtIsNil()).
		WithMembership(func(q *ent.MembershipQuery) { q.WithUser() }).
		Order(ent.Asc(apitoken.FieldID)).
		All(ctx)
}

// RevokePersonal retires the active Personal token id of the Workspace s on behalf of caller: the
// caller's own, or anyone's when the Role holds members:manage. One the caller may not see is
// ErrPersonalTokenNotFound, as an unknown one is.
func RevokePersonal(ctx context.Context, s *ent.Scoped, caller *ent.Membership, id int64) error {
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(id), isPersonalToken(), visibleTo(caller), apitoken.RevokedAtIsNil()).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPersonalTokenNotFound
	}
	return nil
}
