package external

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-faster/jx"
	"github.com/samber/lo"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/transactional"
)

// EmailsSend is the transactional send surface (ADR 0005). The whole operation is the
// transactional module's; this handler only unpacks the request, calls it and renders its
// Result as the HTTP contract. The optional Idempotency-Key becomes the send's claim key: a
// repeated key replays the recorded outcome instead of sending again, and a key whose first
// request is still in flight returns 409.
func (h *Handlers) EmailsSend(ctx context.Context, req *externalapi.SendTransactionalEmailInput, params externalapi.EmailsSendParams) (externalapi.EmailsSendRes, error) {
	res, err := h.transactional.Send(ctx, auth.TokenScoped(ctx), transactional.Request{
		TemplateID:     int64(req.TemplateId),
		Destination:    string(req.Destination),
		Variables:      lo.MapValues(req.Variables.Or(nil), func(v jx.Raw, _ string) json.RawMessage { return json.RawMessage(v) }),
		IdempotencyKey: params.IdempotencyKey.Or(""),
		Track:          req.Track.Or(false),
	})
	var invalid *transactional.InvalidVariablesError
	switch {
	case errors.Is(err, transactional.ErrEmptyDestination):
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "destination must not be empty"))
		return &r, nil
	case errors.Is(err, transactional.ErrTemplateNotFound):
		r := externalapi.EmailsSendNotFound(problem(http.StatusNotFound, "template not found"))
		return &r, nil
	case errors.As(err, &invalid):
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "invalid variables: "+invalid.Error()))
		return &r, nil
	case err != nil:
		return nil, err
	}

	switch res.Status {
	case transactional.Sent:
		return sendResponse(res.MessageID, externalapi.TransactionalSendStatusSent, res.Destination), nil
	case transactional.Suppressed:
		return sendResponse(res.MessageID, externalapi.TransactionalSendStatusSuppressed, res.Destination), nil
	case transactional.InProgress:
		r := externalapi.EmailsSendConflict(problem(http.StatusConflict, "a send with this Idempotency-Key is already in progress"))
		return &r, nil
	case transactional.ReplayedFailed:
		r := externalapi.EmailsSendConflict(problem(http.StatusConflict, "a previous send with this Idempotency-Key failed; retry with a new key"))
		return &r, nil
	case transactional.FailedRender:
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "render template: "+res.Reason))
		return &r, nil
	default: // transactional.Held: a reversible hold on the source, never a failed send
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, outbound.Detail(res.Reason)))
		return &r, nil
	}
}

func sendResponse(id int64, status externalapi.TransactionalSendStatus, dest string) externalapi.EmailsSendRes {
	return &externalapi.SendTransactionalEmailResponse{ID: entityID(id), Status: status, Destination: dest}
}

// entityID renders an int64 primary key as the API's string EntityId.
func entityID(id int64) externalapi.EntityId {
	return externalapi.EntityId(id)
}
