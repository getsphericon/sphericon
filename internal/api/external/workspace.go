package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/providerlimit"
)

// WorkspaceGet returns the Workspace the token belongs to, with the Send cap the platform set on
// it (ADR 0067): null when none applies. Any token may read it; nothing over /api writes it.
func (h *Handlers) WorkspaceGet(ctx context.Context) (externalapi.WorkspaceGetRes, error) {
	s := auth.TokenScoped(ctx)
	ws, err := h.accounts.Workspace(ctx, s)
	if err != nil {
		return nil, err
	}
	sendCap, err := h.outbound.SendCap(ctx, s, ws)
	if err != nil {
		return nil, err
	}
	res := mapper.WorkspaceToResource(ws)
	res.SendCap = sendCapOrNull(sendCap)
	return &res, nil
}

// sendCapOrNull is the contract's nullable Send cap: null when none applies.
func sendCapOrNull(st *providerlimit.SendCapStatus) externalapi.NilSendCap {
	if st == nil {
		return externalapi.NilSendCap{Null: true}
	}
	return externalapi.NewNilSendCap(mapper.SendCapToResource(*st))
}
