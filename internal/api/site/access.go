package site

import (
	"context"

	"github.com/ogen-go/ogen/middleware"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
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
	// whileAcceptancePending lets a signed-in User with a required acceptance pending through
	// (ADR 0062): the operation is marked x-allowed-while-acceptance-pending in TypeSpec, which
	// is where the operations are listed. Every other operation that carries a session refuses
	// them; an anonymous one carries none and is never refused for it. It lifts only the gate: an
	// operation of a Workspace still applies the Two-factor requirement and its Permissions.
	whileAcceptancePending bool
}

// errUndeclared refuses an operation the generated table has no entry for: refused to everyone.
var errUndeclared = validation.Forbidden("site: the operation declares no access", "errors.permission.unknown")

// Access is the ogen middleware of /site (ADR 0061), and the site's construction point of the
// scoped client (ADR 0017). For an operation of a Workspace it loads the caller's Membership of
// the Workspace its workspaceId names; it then admits the signed-in User (accounts.Admit: a
// required acceptance pending, then the Workspace's Two-factor requirement, each unless the
// operation lets it through), checks the Permissions the operation declares and puts the
// Workspace's *ent.Scoped (acting as the caller, ADR 0022) and the Membership in the context,
// where the handler reads them with workspaceScope and callerMembership. A request without a
// session passes to an operation of no Workspace (an anonymous operation, or an optional session
// not given). A Workspace that does not exist or of which the caller is not a member is
// accounts.ErrWorkspaceNotFound (404); a Role without a Permission is its refusal (403
// permission_required). Every refusal is rendered by the server's error handler.
func (h *Handlers) Access(req middleware.Request, next middleware.Next) (middleware.Response, error) {
	a, ok := operationAccess[req.OperationName]
	if !ok {
		return middleware.Response{}, errUndeclared
	}
	session := auth.GetSiteAuth(req.Context)
	if session == nil {
		if a.workspace {
			return middleware.Response{}, accounts.ErrWorkspaceNotFound
		}
		return next(req)
	}
	var c caller
	if a.workspace {
		id, _ := req.Params.Path("workspaceId")
		publicID, _ := id.(siteapi.WorkspaceId)
		s, m, err := h.accounts.Scope(req.Context, session.UserID, string(publicID))
		if err != nil {
			return middleware.Response{}, err
		}
		c = caller{scoped: s, membership: m}
	}
	err := h.accounts.Admit(req.Context, accounts.Admission{
		UserID:                 session.UserID,
		Membership:             c.membership,
		StrongSession:          currentSession(req.Context).Method.Strong(),
		WhileAcceptancePending: a.whileAcceptancePending,
		WhileWithheld:          a.whileWithheld,
		At:                     h.now(),
	})
	if err != nil {
		return middleware.Response{}, err
	}
	if !a.workspace {
		return next(req)
	}
	for _, p := range a.permissions {
		if err := accounts.Require(c.membership.Role, p); err != nil {
			return middleware.Response{}, err
		}
	}
	req.SetContext(context.WithValue(req.Context, workspaceKey{}, c))
	return next(req)
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
