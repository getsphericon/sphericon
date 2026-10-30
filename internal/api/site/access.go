package site

import (
	"context"

	"github.com/ogen-go/ogen/middleware"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/validation"
)

// access is what an operation of /site declares in the contract (ADR 0061); cmd/scopegen
// generates one per operation into operationAccess.
type access struct {
	// workspace is an operation of the Workspace its workspaceId path parameter names.
	workspace bool
	// permissions are what the caller's Role must hold there; none means any member.
	permissions []accounts.Permission
	// whileWithheld lets a caller the Two-factor requirement withholds through (ADR 0020): only
	// switching the requirement off.
	whileWithheld bool
}

// errUndeclared refuses an operation the generated table has no entry for: refused to everyone.
var errUndeclared = validation.Forbidden("site: the operation declares no access", "errors.permission.unknown")

// Access is the ogen middleware of /site (ADR 0061), and the site's construction point of the
// scoped client (ADR 0017). For an operation of a Workspace it loads the caller's Membership of
// the Workspace its workspaceId names, applies the Two-factor requirement, checks the
// Permissions the operation declares and puts the Workspace's *ent.Scoped (acting as the caller,
// ADR 0022) and the Membership in the context, where the handler reads them with workspaceScope
// and callerMembership. A Workspace that does not exist or of which the caller is not a member is
// accounts.ErrWorkspaceNotFound (404); a Role without a Permission is its refusal (403
// permission_required). Every refusal is rendered by the server's error handler.
func (h *Handlers) Access(req middleware.Request, next middleware.Next) (middleware.Response, error) {
	a, ok := operationAccess[req.OperationName]
	if !ok {
		return middleware.Response{}, errUndeclared
	}
	if !a.workspace {
		return next(req)
	}
	id, _ := req.Params.Path("workspaceId")
	publicID, _ := id.(siteapi.WorkspaceId)
	s, m, err := h.member(req.Context, string(publicID), a.whileWithheld)
	if err != nil {
		return middleware.Response{}, err
	}
	for _, p := range a.permissions {
		if err := accounts.Require(m.Role, p); err != nil {
			return middleware.Response{}, err
		}
	}
	req.SetContext(context.WithValue(req.Context, workspaceKey{}, caller{scoped: s, membership: m}))
	return next(req)
}

// member is the signed-in User's Membership of the Workspace publicID names (its User and
// Workspace loaded) and the Workspace's scoped client acting as that User. Once the User's grace
// under the Workspace's Two-factor requirement has ended without a Second factor it is
// secondFactorRequiredError (403 second_factor_required), unless whileWithheld; other
// Workspaces and the /me endpoints are unaffected.
func (h *Handlers) member(ctx context.Context, publicID string, whileWithheld bool) (*ent.Scoped, *ent.Membership, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, nil, accounts.ErrWorkspaceNotFound
	}
	s, m, err := h.accounts.Scope(ctx, a.UserID, publicID)
	if err != nil {
		return nil, nil, err
	}
	if !whileWithheld && secondfactor.Withheld(m, h.now(), currentSession(ctx).Method.Strong()) {
		return nil, nil, secondFactorRequiredError{}
	}
	return s, m, nil
}

// workspaceKey is the context key of the caller Access resolved.
type workspaceKey struct{}

// caller is the Workspace an operation addresses, as Access resolved it.
type caller struct {
	scoped     *ent.Scoped
	membership *ent.Membership
}

// workspaceScope is the scoped client of the Workspace the operation addresses, put in the
// context by Access. Only an operation of a Workspace has one.
func workspaceScope(ctx context.Context) *ent.Scoped {
	c, _ := ctx.Value(workspaceKey{}).(caller)
	return c.scoped
}

// callerMembership is the caller's Membership of the Workspace the operation addresses (its User
// and Workspace loaded), put in the context by Access.
func callerMembership(ctx context.Context) *ent.Membership {
	c, _ := ctx.Value(workspaceKey{}).(caller)
	return c.membership
}
