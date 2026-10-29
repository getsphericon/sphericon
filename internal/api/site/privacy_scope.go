package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/erasure"
)

// privacyScope is the step the Privacy request operations (ADR 0021) share: the Workspace
// scope, the owner-or-admin gate and a valid email address. A non-nil refusal is the
// problem to answer with, its Status naming the response (403 or 400); the operations
// have distinct generated response types, so each converts it itself. A Workspace the
// caller cannot reach is the error accounts.ErrWorkspaceNotFound.
func (h *Handlers) privacyScope(ctx context.Context, workspaceID siteapi.WorkspaceId, email string) (*ent.Scoped, *siteapi.ProblemDetails, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if !accounts.CanErase(role) {
		p := problem(http.StatusForbidden, "insufficient role")
		return nil, &p, nil
	}
	_, err = erasure.NormalizeEmail(email)
	if errors.Is(err, erasure.ErrInvalidEmail) {
		p := problem(http.StatusBadRequest, "invalid email address")
		return nil, &p, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return scoped, nil, nil
}
