package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// The Captured message operations are thin adapters over internal/capturedmessages, the module
// /site shares. Reading needs captured_messages:read, deleting captured_messages:write.

// CapturedMessagesList returns the Workspace's Captured messages, newest first, without content.
func (h *Handlers) CapturedMessagesList(ctx context.Context, params externalapi.CapturedMessagesListParams) (externalapi.CapturedMessagesListRes, error) {
	page, err := h.captured.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	out := mapper.CapturedMessagesListPage(page)
	return &out, nil
}

// CapturedMessagesGet returns one Captured message in full.
func (h *Handlers) CapturedMessagesGet(ctx context.Context, params externalapi.CapturedMessagesGetParams) (externalapi.CapturedMessagesGetRes, error) {
	row, err := h.captured.Get(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.CapturedMessageToResource(row)
	return &res, nil
}

// CapturedMessagesDelete deletes one Captured message.
func (h *Handlers) CapturedMessagesDelete(ctx context.Context, params externalapi.CapturedMessagesDeleteParams) (externalapi.CapturedMessagesDeleteRes, error) {
	err := h.captured.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &externalapi.CapturedMessagesDeleteNoContent{}, nil
}

// CapturedMessagesDeleteAll deletes every Captured message of the Workspace.
func (h *Handlers) CapturedMessagesDeleteAll(ctx context.Context) (externalapi.CapturedMessagesDeleteAllRes, error) {
	if err := h.captured.DeleteAll(ctx, auth.TokenScoped(ctx)); err != nil {
		return nil, err
	}
	return &externalapi.CapturedMessagesDeleteAllNoContent{}, nil
}
