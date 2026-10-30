// Package apitokens owns minting and revoking workspace API tokens. Every entry
// point that creates one (the /api token endpoints, bootstrap, the /site settings
// screen) calls it, so the rules are written once: a name is required, scopes come
// from the contract's vocabulary, and a token minted by another token can never
// carry a scope its minter lacks. Handlers stay thin adapters: authorization of
// the caller (scope, role), call, map errors.
//
// OAuth issuing (internal/oauthserver) keeps its own flow: its scopes are fixed by
// the consent screen, and it only shares the credential generators in
// internal/credentials. What it issues is a Connection, an API token tied to the
// approving User's Membership (ADR 0061): the Workspace's API tokens below are the rows
// without one, and a Connection is listed and revoked through Connections and
// RevokeConnection instead.
package apitokens

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

var (
	// ErrScopeEscalation: a token asked to mint a scope it does not hold itself.
	ErrScopeEscalation = errors.New("apitokens: cannot grant a scope the minting token lacks")
	// ErrNotFound: no such token in this Workspace (unknown or foreign); for Revoke and
	// Rename, also one already revoked.
	ErrNotFound = errors.New("apitokens: token not found")
)

// nameEmpty refuses a token name that is blank after trimming.
func nameEmpty() *validation.Error {
	return &validation.Error{Field: "name", Message: i18n.T("errors.name_empty", nil)}
}

// unknownScope refuses a scope that is not in the vocabulary (Scopes).
func unknownScope(scope string) *validation.Error {
	return &validation.Error{Field: "scopes", Message: i18n.T("errors.token_scope_unknown", map[string]any{"Scope": scope})}
}

// Input is what a caller chooses when minting.
type Input struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// Minted is a created token and its full secret value, shown to the caller once;
// only a SHA-256 hash and the public prefix are stored.
type Minted struct {
	Token *ent.ApiToken
	Value string
}

// Mint creates a token with any scope of the vocabulary. The caller has already
// been authorized to hold that power: a Membership whose Role holds tokens:manage on /site,
// or the bootstrap secret. A token minting another token goes through MintWithin.
func Mint(ctx context.Context, s *ent.Scoped, in Input) (Minted, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Minted{}, nameEmpty()
	}
	scopes := slices.Compact(slices.Sorted(slices.Values(in.Scopes)))
	for _, sc := range scopes {
		if !validScope(sc) {
			return Minted{}, unknownScope(sc)
		}
	}

	prefix, err := credentials.GenerateTokenPrefix()
	if err != nil {
		return Minted{}, err
	}
	secret, err := credentials.GenerateTokenSecret()
	if err != nil {
		return Minted{}, err
	}

	create := s.ApiToken().Create().
		SetName(name).
		SetPrefix(prefix).
		SetSecretHash(credentials.HashSecret(secret)).
		SetScopes(scopes)
	if in.ExpiresAt != nil {
		create = create.SetExpiresAt(*in.ExpiresAt)
	}
	token, err := create.Save(ctx)
	if err != nil {
		return Minted{}, err
	}
	return Minted{Token: token, Value: credentials.TokenValue(prefix, secret)}, nil
}

// MintWithin is Mint for a token acting as minter: every requested scope must be
// among the minter's own, so tokens:write cannot be turned into send or any other
// power the minter does not already have.
func MintWithin(ctx context.Context, s *ent.Scoped, minter []string, in Input) (Minted, error) {
	for _, sc := range in.Scopes {
		if !slices.Contains(minter, sc) {
			return Minted{}, ErrScopeEscalation
		}
	}
	return Mint(ctx, s, in)
}

// List returns the Workspace's API tokens in display order, oldest first; Connections are
// not among them. A revoked token stays listed, marked by its revoked_at. Both the /site
// settings screen and the /api token read call it, so the shape of the query is written once.
func List(ctx context.Context, s *ent.Scoped) ([]*ent.ApiToken, error) {
	return s.ApiToken().Query().Where(apitoken.MembershipIDIsNil()).Order(ent.Asc(apitoken.FieldID)).All(ctx)
}

// Get returns the Workspace's token with the id, revoked or not and Connection or not (the
// credential a request presents, read back by /api's "me"), or ErrNotFound when the Workspace
// has none with that id.
func Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.ApiToken, error) {
	token, err := s.ApiToken().Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return token, nil
}

// Revoke retires an active API token (it stays listed with revoked_at). A token that
// is unknown, belongs to another Workspace, is a Connection or is already revoked is ErrNotFound.
func Revoke(ctx context.Context, s *ent.Scoped, id int64) error {
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(id), apitoken.MembershipIDIsNil(), apitoken.RevokedAtIsNil()).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Rename changes the name of an active API token and returns the updated row. A token that
// is unknown, belongs to another Workspace, is a Connection or is revoked is ErrNotFound; a
// blank name is a *validation.Error on `name`.
func Rename(ctx context.Context, s *ent.Scoped, id int64, name string) (*ent.ApiToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nameEmpty()
	}
	n, err := s.ApiToken().Update().
		Where(apitoken.ID(id), apitoken.MembershipIDIsNil(), apitoken.RevokedAtIsNil()).
		SetName(name).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.ApiToken().Get(ctx, id)
}

func validScope(scope string) bool {
	return slices.Contains(Scopes, Scope(scope))
}
