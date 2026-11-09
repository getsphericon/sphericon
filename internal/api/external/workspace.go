package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// WorkspaceGet is the Workspace the token acts in, with the Send cap the send path enforces on it
// (ADR 0067) and its sends over the last 24 hours; the Send cap is null when none applies.
func (h *Handlers) WorkspaceGet(ctx context.Context) (*externalapi.WorkspaceResource, error) {
	sum, err := h.workspaceView.Of(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	res := mapper.WorkspaceToResource(sum.Workspace)
	res.SendCap = mapper.SendCapStatusToResource(sum.SendCap)
	return &res, nil
}
