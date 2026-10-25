package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/capturedmessages"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// The Captured message operations are thin adapters over internal/capturedmessages, the module
// /site shares. Reading needs captured_messages:read, deleting captured_messages:write.

// CapturedMessagesList returns the Workspace's Captured messages, newest first, without content.
func (h *Handlers) CapturedMessagesList(ctx context.Context, params externalapi.CapturedMessagesListParams) (externalapi.CapturedMessagesListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeCapturedMessagesRead) {
		res := externalapi.CapturedMessagesListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	page, err := h.captured.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]externalapi.CapturedMessageSummary, len(page.Items))
	for i, m := range page.Items {
		items[i] = mapper.CapturedMessageToSummary(m)
	}
	return &externalapi.CapturedMessagesListOK{
		Items:      items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// CapturedMessagesGet returns one Captured message in full.
func (h *Handlers) CapturedMessagesGet(ctx context.Context, params externalapi.CapturedMessagesGetParams) (externalapi.CapturedMessagesGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeCapturedMessagesRead) {
		res := externalapi.CapturedMessagesGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	row, err := h.captured.Get(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if errors.Is(err, capturedmessages.ErrNotFound) {
		res := externalapi.CapturedMessagesGetNotFound(problem(http.StatusNotFound, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.CapturedMessageToResource(row)
	return &res, nil
}

// CapturedMessagesDelete deletes one Captured message.
func (h *Handlers) CapturedMessagesDelete(ctx context.Context, params externalapi.CapturedMessagesDeleteParams) (externalapi.CapturedMessagesDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeCapturedMessagesWrite) {
		res := externalapi.CapturedMessagesDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	err := h.captured.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if errors.Is(err, capturedmessages.ErrNotFound) {
		res := externalapi.CapturedMessagesDeleteNotFound(problem(http.StatusNotFound, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.CapturedMessagesDeleteNoContent{}, nil
}

// CapturedMessagesDeleteAll deletes every Captured message of the Workspace.
func (h *Handlers) CapturedMessagesDeleteAll(ctx context.Context) (externalapi.CapturedMessagesDeleteAllRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeCapturedMessagesWrite) {
		res := problem(http.StatusUnauthorized, "insufficient scope")
		return &res, nil
	}
	if err := h.captured.DeleteAll(ctx, auth.TokenScoped(ctx)); err != nil {
		return nil, err
	}
	return &externalapi.CapturedMessagesDeleteAllNoContent{}, nil
}
