package external

import (
	"context"

	"github.com/getsphericon/sphericon/ent/contact"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

func (h *Handlers) ContactsList(ctx context.Context, params externalapi.ContactsListParams) (externalapi.ContactsListRes, error) {
	page, err := h.contacts.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), contacts.Filter{})
	if err != nil {
		return nil, err
	}

	out := mapper.ContactsListPage(page)
	return &out, nil
}

func (h *Handlers) ContactsCreate(ctx context.Context, req *externalapi.CreateContactInput) (externalapi.ContactsCreateRes, error) {
	c, err := h.contacts.Create(ctx, auth.TokenScoped(ctx), contact.SourceAPI, inputs.ContactCreate(req))
	if err != nil {
		return nil, err
	}

	resource := mapper.ContactToResource(c)
	return &resource, nil
}

func (h *Handlers) ContactsGet(ctx context.Context, params externalapi.ContactsGetParams) (externalapi.ContactsGetRes, error) {
	id := int64(params.ID)

	c, err := h.contacts.Get(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}

	resource := mapper.ContactToResource(c)
	return &resource, nil
}

func (h *Handlers) ContactsUpdate(ctx context.Context, req *externalapi.UpdateContactInput, params externalapi.ContactsUpdateParams) (externalapi.ContactsUpdateRes, error) {
	id := int64(params.ID)

	c, err := h.contacts.Update(ctx, auth.TokenScoped(ctx), id, inputs.ContactUpdate(req))
	if err != nil {
		return nil, err
	}

	resource := mapper.ContactToResource(c)
	return &resource, nil
}

// tokenRequester is the API token behind the request, as the requester of an Erasure.
func tokenRequester(ctx context.Context) erasure.Requester {
	return erasure.Requester{Kind: erasure.RequesterAPIToken, ID: auth.GetTokenAuth(ctx).TokenID}
}

// ContactsDelete is Erasure (ADR 0021): deleting a Contact removes its personal data.
// It has its own scope, separate from contacts:write, because it is irreversible.
func (h *Handlers) ContactsDelete(ctx context.Context, params externalapi.ContactsDeleteParams) (externalapi.ContactsDeleteRes, error) {
	id := int64(params.ID)

	err := h.erasure.Erase(ctx, auth.TokenScoped(ctx), erasure.ByContactID(id),
		tokenRequester(ctx))
	if err != nil {
		return nil, err
	}
	return &externalapi.ContactsDeleteNoContent{}, nil
}

// ContactsEraseBy is Erasure by an email address or a visitor id (ADR 0021), under the
// same scope and rules as deleting by id. Exactly one identifier is required: the erasure
// module refuses anything else.
func (h *Handlers) ContactsEraseBy(ctx context.Context, params externalapi.ContactsEraseByParams) (externalapi.ContactsEraseByRes, error) {
	id, err := erasure.ByEmailOrVisitorID(convert.StringPtr(params.Email), convert.StringPtr(params.VisitorId))
	if err != nil {
		return nil, err
	}
	if err := h.erasure.Erase(ctx, auth.TokenScoped(ctx), id, tokenRequester(ctx)); err != nil {
		return nil, err
	}
	return &externalapi.ContactsEraseByNoContent{}, nil
}

// ContactsBatchUpsert upserts each Contact independently and reports a per-item result.
func (h *Handlers) ContactsBatchUpsert(ctx context.Context, req *externalapi.UpsertContactsInput) (externalapi.ContactsBatchUpsertRes, error) {
	outcomes := h.contacts.UpsertBatch(ctx, auth.TokenScoped(ctx), contact.SourceAPI, inputs.ContactUpserts(req.Contacts))

	results := make([]externalapi.ContactBatchItemResult, len(outcomes))
	for i, o := range outcomes {
		results[i].Index = int32(i)
		switch {
		case o.Err != nil:
			results[i].Status = externalapi.ContactBatchStatusFailed
			results[i].Error = externalapi.NewOptString(itemError(o.Err))
		case o.Result.Created:
			results[i].Status = externalapi.ContactBatchStatusCreated
		default:
			results[i].Status = externalapi.ContactBatchStatusUpdated
		}
		if o.Err == nil {
			results[i].ContactId = externalapi.NewOptEntityId(externalapi.EntityId(o.Result.Contact.ID))
		}
	}
	return &externalapi.UpsertContactsResult{Results: results}, nil
}

// itemError words a batch item's failure for the caller: a refusal of a module as its own
// message, anything else (storage) as a generic one so internals do not leak.
func itemError(err error) string {
	if shown, ok := validation.Shown(err); ok {
		return shown
	}
	return "internal error"
}
