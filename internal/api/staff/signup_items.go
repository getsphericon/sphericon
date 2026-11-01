package staff

import (
	"context"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/staffmembers"
)

// StaffSignupItemsList is every Sign-up item of the Instance, in the order of the sign-up form.
func (h *Handlers) StaffSignupItemsList(ctx context.Context) (staffapi.StaffSignupItemsListRes, error) {
	items, err := h.items.List(ctx)
	if err != nil {
		return nil, err
	}
	return &staffapi.StaffSignupItemList{Items: mapper.SignupItemsToResources(items)}, nil
}

// StaffSignupItemsCreate adds a Sign-up item.
func (h *Handlers) StaffSignupItemsCreate(ctx context.Context, req *staffapi.StaffSignupItemInput) (staffapi.StaffSignupItemsCreateRes, error) {
	it, err := h.items.Add(ctx, inputs.SignupItemCreate(req))
	if err != nil {
		return nil, err
	}
	return new(mapper.SignupItemToResource(it)), nil
}

// StaffSignupItemsGet is one Sign-up item.
func (h *Handlers) StaffSignupItemsGet(ctx context.Context, params staffapi.StaffSignupItemsGetParams) (staffapi.StaffSignupItemsGetRes, error) {
	it, err := h.items.Get(ctx, int64(params.ID))
	if err != nil {
		return nil, err
	}
	return new(mapper.SignupItemToResource(it)), nil
}

// StaffSignupItemsUpdate changes a Sign-up item.
func (h *Handlers) StaffSignupItemsUpdate(ctx context.Context, req *staffapi.StaffSignupItemUpdate, params staffapi.StaffSignupItemsUpdateParams) (staffapi.StaffSignupItemsUpdateRes, error) {
	it, err := h.items.Update(ctx, int64(params.ID), inputs.SignupItemUpdate(req))
	if err != nil {
		return nil, err
	}
	return new(mapper.SignupItemToResource(it)), nil
}

// StaffSignupItemsReorder puts the Sign-up items in a new order.
func (h *Handlers) StaffSignupItemsReorder(ctx context.Context, req *staffapi.StaffSignupItemOrder) (staffapi.StaffSignupItemsReorderRes, error) {
	items, err := h.items.Reorder(ctx, inputs.SignupItemOrder(req.Ids))
	if err != nil {
		return nil, err
	}
	return &staffapi.StaffSignupItemList{Items: mapper.SignupItemsToResources(items)}, nil
}

// StaffSignupItemsArchive archives a Sign-up item.
func (h *Handlers) StaffSignupItemsArchive(ctx context.Context, params staffapi.StaffSignupItemsArchiveParams) (staffapi.StaffSignupItemsArchiveRes, error) {
	it, err := h.items.Archive(ctx, int64(params.ID))
	if err != nil {
		return nil, err
	}
	return new(mapper.SignupItemToResource(it)), nil
}

// StaffSignupItemsListVersions is every published version of the item's Legal document.
func (h *Handlers) StaffSignupItemsListVersions(ctx context.Context, params staffapi.StaffSignupItemsListVersionsParams) (staffapi.StaffSignupItemsListVersionsRes, error) {
	versions, err := h.items.Versions(ctx, int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &staffapi.StaffLegalDocumentVersionList{Items: mapper.VersionsToResources(versions)}, nil
}

// StaffSignupItemsSaveDraft saves the draft of the item's Legal document.
func (h *Handlers) StaffSignupItemsSaveDraft(ctx context.Context, req *staffapi.StaffLegalDocumentDraftInput, params staffapi.StaffSignupItemsSaveDraftParams) (staffapi.StaffSignupItemsSaveDraftRes, error) {
	it, err := h.items.SaveDraft(ctx, int64(params.ID), string(req.Markdown))
	if err != nil {
		return nil, err
	}
	return new(mapper.SignupItemToResource(it)), nil
}

// StaffSignupItemsPublish publishes the draft as a new version, recorded with the signed-in Staff
// member.
func (h *Handlers) StaffSignupItemsPublish(ctx context.Context, req *staffapi.StaffLegalDocumentPublishInput, params staffapi.StaffSignupItemsPublishParams) (staffapi.StaffSignupItemsPublishRes, error) {
	it, err := h.items.Publish(ctx, int64(params.ID), string(req.Label), staffmembers.Current(ctx).ID)
	if err != nil {
		return nil, err
	}
	return new(mapper.SignupItemToResource(it)), nil
}
