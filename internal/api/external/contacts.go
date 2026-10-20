package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) ContactsList(ctx context.Context, params externalapi.ContactsListParams) (externalapi.ContactsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:read") {
		res := externalapi.ContactsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.contacts.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), "")
	if err != nil {
		return nil, err
	}

	return &externalapi.ContactsListOK{
		Items: lo.Map(page.Items, func(c *ent.Contact, _ int) externalapi.ContactResource {
			return mapper.ContactToResource(c)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) ContactsCreate(ctx context.Context, req *externalapi.CreateContactInput) (externalapi.ContactsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:write") {
		res := externalapi.ContactsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	c, err := h.contacts.Create(ctx, auth.TokenScoped(ctx), createContactAttributes(req))
	if conflict, ok := errors.AsType[*contacts.ConflictError](err); ok {
		res := externalapi.ContactsCreateConflict(conflictProblem(conflict))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}

	resource := mapper.ContactToResource(c)
	return &resource, nil
}

func (h *Handlers) ContactsGet(ctx context.Context, params externalapi.ContactsGetParams) (externalapi.ContactsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:read") {
		res := externalapi.ContactsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	c, err := auth.TokenScoped(ctx).Contact().Get(ctx, id)
	if ent.IsNotFound(err) {
		res := externalapi.ContactsGetNotFound(problem(http.StatusNotFound, "contact not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}

	resource := mapper.ContactToResource(c)
	return &resource, nil
}

func (h *Handlers) ContactsUpdate(ctx context.Context, req *externalapi.UpdateContactInput, params externalapi.ContactsUpdateParams) (externalapi.ContactsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:write") {
		res := externalapi.ContactsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	c, err := h.contacts.Update(ctx, auth.TokenScoped(ctx), id, updateContactAttributes(req))
	if conflict, ok := errors.AsType[*contacts.ConflictError](err); ok {
		res := externalapi.ContactsUpdateConflict(conflictProblem(conflict))
		return &res, nil
	}
	if errors.Is(err, contacts.ErrNotFound) {
		res := externalapi.ContactsUpdateNotFound(problem(http.StatusNotFound, "contact not found"))
		return &res, nil
	}
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
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:erase") {
		res := externalapi.ContactsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	err := h.erasure.Erase(ctx, auth.TokenScoped(ctx), erasure.ByContactID(id),
		tokenRequester(ctx))
	if errors.Is(err, erasure.ErrNotFound) {
		res := externalapi.ContactsDeleteNotFound(problem(http.StatusNotFound, "contact not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.ContactsDeleteNoContent{}, nil
}

// ContactsEraseBy is Erasure by an email address or a visitor id (ADR 0021), under the
// same scope and rules as deleting by id. Exactly one identifier is required.
func (h *Handlers) ContactsEraseBy(ctx context.Context, params externalapi.ContactsEraseByParams) (externalapi.ContactsEraseByRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:erase") {
		res := externalapi.ContactsEraseByUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	email, hasEmail := params.Email.Get()
	visitorID, hasVisitor := params.VisitorId.Get()
	if hasEmail == hasVisitor || (hasVisitor && visitorID == "") {
		res := externalapi.ContactsEraseByBadRequest(problem(http.StatusBadRequest, "pass exactly one of email or visitorId"))
		return &res, nil
	}
	id := erasure.ByVisitorID(visitorID)
	if hasEmail {
		id = erasure.ByEmail(string(email))
	}

	err := h.erasure.Erase(ctx, auth.TokenScoped(ctx), id,
		tokenRequester(ctx))
	if errors.Is(err, erasure.ErrNotFound) {
		res := externalapi.ContactsEraseByNotFound(problem(http.StatusNotFound, "nothing found for the identifier"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.ContactsEraseByNoContent{}, nil
}

// ContactsBatchUpsert upserts each Contact independently and reports a per-item result.
func (h *Handlers) ContactsBatchUpsert(ctx context.Context, req *externalapi.UpsertContactsInput) (externalapi.ContactsBatchUpsertRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "contacts:write") {
		res := externalapi.ContactsBatchUpsertUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	items := make([]contacts.Attributes, len(req.Contacts))
	for i, c := range req.Contacts {
		items[i] = contacts.Attributes{
			SubjectID: convert.StringPtr(c.SubjectId),
			Email:     convert.StringPtr(c.Email),
			Phone:     convert.StringPtr(c.Phone),
			FirstName: convert.StringPtr(c.FirstName),
			LastName:  convert.StringPtr(c.LastName),
			TimeZone:  convert.StringPtr(c.TimeZone),
		}
		if v, ok := c.CustomFields.Get(); ok {
			items[i].CustomFields = convert.RawMap(v)
		}
	}
	outcomes := h.contacts.UpsertBatch(ctx, auth.TokenScoped(ctx), items)

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

// itemError words a batch item's failure for the caller: domain errors as their own
// message, anything else (storage) as a generic one so internals do not leak.
func itemError(err error) string {
	var conflict *contacts.ConflictError
	_, rejected := events.AsRejection(err)
	switch {
	case errors.As(err, &conflict):
		return conflict.Message()
	case errors.Is(err, contacts.ErrIdentityRequired), rejected:
		return err.Error()
	default:
		return "internal error"
	}
}

// conflictProblem renders a contacts.ConflictError, with the field-level error.
func conflictProblem(c *contacts.ConflictError) externalapi.ProblemDetails {
	p := problem(http.StatusConflict, c.Message())
	p.Errors = externalapi.NewOptProblemDetailsErrors(externalapi.ProblemDetailsErrors{c.Field: {c.Message()}})
	return p
}

func createContactAttributes(req *externalapi.CreateContactInput) contacts.Attributes {
	attrs := contacts.Attributes{
		SubjectID: convert.StringPtr(req.SubjectId),
		Email:     convert.StringPtr(req.Email),
		Phone:     convert.StringPtr(req.Phone),
		FirstName: convert.StringPtr(req.FirstName),
		LastName:  convert.StringPtr(req.LastName),
		TimeZone:  convert.StringPtr(req.TimeZone),
	}
	if v, ok := req.CustomFields.Get(); ok {
		attrs.CustomFields = convert.RawMap(v)
	}
	return attrs
}

func updateContactAttributes(req *externalapi.UpdateContactInput) contacts.Attributes {
	attrs := contacts.Attributes{
		SubjectID: convert.StringPtr(req.SubjectId),
		Email:     convert.StringPtr(req.Email),
		Phone:     convert.StringPtr(req.Phone),
		FirstName: convert.StringPtr(req.FirstName),
		LastName:  convert.StringPtr(req.LastName),
		TimeZone:  convert.StringPtr(req.TimeZone),
		// JSON Merge Patch: an explicit null clears the field, an absent key keeps it.
		Cleared: contacts.Cleared{
			SubjectID:    req.SubjectId.IsNull(),
			Email:        req.Email.IsNull(),
			Phone:        req.Phone.IsNull(),
			FirstName:    req.FirstName.IsNull(),
			LastName:     req.LastName.IsNull(),
			TimeZone:     req.TimeZone.IsNull(),
			CustomFields: req.CustomFields.IsNull(),
		},
	}
	if v, ok := req.CustomFields.Get(); ok {
		attrs.CustomFields = convert.RawMap(v)
	}
	return attrs
}
