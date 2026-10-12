package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/oklog/ulid/v2"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/templates"
)

// EmailsSend is the transactional send surface (ADR 0005): a single-recipient
// email rendered from a referenced Template with per-call variables at send time.
// It binds the template by reference (the Template's current content is handed to
// Outbound send), and carries no sending source, so it skips Unsubscribe but still
// respects Suppression.
//
// Everything between "this email is wanted" and "the provider accepted it" —
// idempotency, the freeze check, the Sending-domain gate, eligibility, rendering,
// the provider call and the send record — is the Outbound send module's
// (ADR 0015). This handler only resolves the request into a Request and maps the
// Outcome to the HTTP contract. The optional Idempotency-Key becomes the module's
// claim key: a repeated key replays the recorded outcome instead of sending again,
// and a key whose first request is still in flight returns 409.
func (h *Handlers) EmailsSend(ctx context.Context, req *externalapi.SendTransactionalEmailInput, params externalapi.EmailsSendParams) (externalapi.EmailsSendRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "emails:send") {
		res := externalapi.EmailsSendUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	dest := eligibility.NormalizeDestination(string(req.Destination))
	if dest == "" {
		res := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "destination must not be empty"))
		return &res, nil
	}

	templateID, err := parseEntityID(req.TemplateId)
	if err != nil {
		res := externalapi.EmailsSendNotFound(problem(http.StatusNotFound, "template not found"))
		return &res, nil
	}
	// Workspace-scoped: another workspace's template id must 404, never send.
	tmpl, err := h.templates.Get(ctx, auth.TokenScoped(ctx), templateID)
	if errors.Is(err, templates.ErrNotFound) {
		res := externalapi.EmailsSendNotFound(problem(http.StatusNotFound, "template not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}

	vars, err := decodeVariables(req.Variables)
	if err != nil {
		res := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "invalid variables: "+err.Error()))
		return &res, nil
	}

	// The contact this destination resolves to, when one exists (transactional mail
	// may go to an address with no contact); the send fact attaches to it.
	contactID, err := contacts.ResolveID(ctx, auth.TokenScoped(ctx), "", &dest, nil)
	if err != nil {
		return nil, err
	}

	// A client key makes the send idempotent; without one every call is its own send.
	key := "transactional:auto:" + ulid.Make().String()
	if k, ok := params.IdempotencyKey.Get(); ok && k != "" {
		key = "transactional:" + k
	}

	res, err := h.outbound.Send(ctx, auth.TokenScoped(ctx), outbound.Request{
		Kind:        outboundmessage.KindTransactional,
		Key:         key,
		Destination: dest,
		ContactID:   contactID,
		Subject:     tmpl.Subject,
		Body:        tmpl.Body,
		Variables:   vars,
		Ref:         outbound.Ref{TemplateID: templateID},
	})
	if errors.Is(err, outbound.ErrInProgress) {
		r := externalapi.EmailsSendConflict(problem(http.StatusConflict, "a send with this Idempotency-Key is already in progress"))
		return &r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("transactional send: %w", err)
	}

	switch res.Outcome {
	case outbound.Sent:
		return sendResponse(res.MessageID, externalapi.TransactionalSendStatusSent, dest), nil
	case outbound.Skipped:
		// Transactional skips only on Suppression (the global hard floor).
		return sendResponse(res.MessageID, externalapi.TransactionalSendStatusSuppressed, dest), nil
	case outbound.Failed:
		if res.Replayed {
			// A failed key is spent: the caller should retry under a fresh key.
			r := externalapi.EmailsSendConflict(problem(http.StatusConflict, "a previous send with this Idempotency-Key failed; retry with a new key"))
			return &r, nil
		}
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "render template: "+res.Reason))
		return &r, nil
	case outbound.Deferral:
		// Transactional is neither reserved against the Send rate limit nor deferred on a
		// provider "too fast" reply (ADR 0023), so this is unreachable here; refuse
		// reversibly rather than misreport a failed send should that ever change.
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, "the send rate limit is spent; retry shortly"))
		return &r, nil
	default: // outbound.Held: a reversible hold on the source, never a failed send
		r := externalapi.EmailsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, outbound.HoldDetail(res.Reason)))
		return &r, nil
	}
}

func sendResponse(id int64, status externalapi.TransactionalSendStatus, dest string) externalapi.EmailsSendRes {
	return &externalapi.SendTransactionalEmailResponse{ID: entityID(id), Status: status, Destination: dest}
}

// entityID renders an int64 primary key as the API's string EntityId.
func entityID(id int64) externalapi.EntityId {
	return externalapi.EntityId(strconv.FormatInt(id, 10))
}

// decodeVariables turns the per-call variables (raw JSON values) into the binding
// map Liquid renders against. Each value is decoded to its natural Go type
// (string/number/bool/nested) so merge tags like {{ amount }} render correctly.
func decodeVariables(opt externalapi.OptSendTransactionalEmailInputVariables) (map[string]any, error) {
	raw, ok := opt.Get()
	if !ok || len(raw) == 0 {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		var val any
		if err := json.Unmarshal([]byte(v), &val); err != nil {
			return nil, fmt.Errorf("variable %q: %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}
