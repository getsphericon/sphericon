package external

import (
	"context"
	"encoding/json"

	"github.com/go-faster/jx"
	"github.com/samber/lo"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/transactional"
)

// EmailsSend is the transactional send surface (ADR 0005). The whole operation is the
// transactional module's; this handler only unpacks the request, calls it and renders its
// Result as the HTTP contract; a request the module refuses is its error. The optional
// Idempotency-Key becomes the send's claim key: a repeated key replays the recorded outcome
// instead of sending again, and a key whose first request is still in flight is refused (409).
func (h *Handlers) EmailsSend(ctx context.Context, req *externalapi.SendTransactionalEmailInput, params externalapi.EmailsSendParams) (externalapi.EmailsSendRes, error) {
	res, err := h.transactional.Send(ctx, auth.TokenScoped(ctx), transactional.Request{
		TemplateID:     int64(req.TemplateId),
		Destination:    string(req.Destination),
		Variables:      lo.MapValues(req.Variables.Or(nil), func(v jx.Raw, _ string) json.RawMessage { return json.RawMessage(v) }),
		IdempotencyKey: params.IdempotencyKey.Or(""),
		Track:          req.Track.Or(false),
	})
	if err != nil {
		return nil, err
	}
	return &externalapi.SendTransactionalEmailResponse{
		ID: entityID(res.MessageID), Status: mapper.TransactionalSendStatusToResource(res.Status), Destination: res.Destination,
	}, nil
}

// entityID renders an int64 primary key as the API's string EntityId.
func entityID(id int64) externalapi.EntityId {
	return externalapi.EntityId(id)
}
