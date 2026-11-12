package outboundmessages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The refusals of a Transactional send that cannot be sent as asked. A send the Outbound send holds
// is ErrHeld, and a Template that cannot render with these variables is
// emailrender.ErrInvalidContent, each with its reason.
var (
	// ErrTransactionalEmptyDestination: the address is empty once normalized.
	ErrTransactionalEmptyDestination = validation.Refuse("transactional: destination must not be empty", i18n.ErrorsDestinationEmpty)
	// ErrTransactionalTemplateNotFound: no such Template in the Workspace.
	ErrTransactionalTemplateNotFound = validation.NotFound("transactional: template not found", i18n.ErrorsTemplateNotFound)
	// ErrTransactionalInvalidVariables: a per-call variable is not valid JSON; it is made With the
	// variable and why.
	ErrTransactionalInvalidVariables = validation.Refuse("transactional: invalid variables", i18n.ErrorsVariablesInvalid)
	// ErrTransactionalInProgress: a send with this Idempotency-Key is still in flight.
	ErrTransactionalInProgress = validation.Conflict("transactional: a send with this key is in progress", i18n.ErrorsSendInProgress)
	// ErrTransactionalReplayedFailed: the Idempotency-Key was spent by a send that failed; retry
	// under a new key.
	ErrTransactionalReplayedFailed = validation.Conflict("transactional: a send with this key failed", i18n.ErrorsSendKeyFailed)
	// ErrTransactionalKeyReused: the Idempotency-Key was spent by a send made for a different
	// request (another Template, destination, variables or tracking); retry under a new key.
	ErrTransactionalKeyReused = validation.Refuse("transactional: the key was used for a different request", i18n.ErrorsSendKeyReused)
	// ErrTransactionalInvariant: the Outbound send answered with something a Transactional request
	// can never get. It is a bug, reported loudly, not a refusal the caller can act on.
	ErrTransactionalInvariant = errors.New("transactional: invariant violated")
)

// TransactionalStatus is how a Transactional send that was not refused ended.
type TransactionalStatus string

const (
	// TransactionalSent: the provider accepted the message.
	TransactionalSent TransactionalStatus = "sent"
	// TransactionalSuppressed: Suppression, the global hard floor, forbids the destination.
	TransactionalSuppressed TransactionalStatus = "suppressed"
)

// TransactionalResult is the answer to a TransactionalRequest.
type TransactionalResult struct {
	Status TransactionalStatus
	// MessageID is the Outbound message of the request.
	MessageID int64
	// Destination is the normalized address.
	Destination string
}

// TransactionalRequest is one Transactional send (GLOSSARY, ADR 0005). Variables are raw JSON
// values, decoded here to their natural Go types so merge tags like {{ amount }} render correctly.
type TransactionalRequest struct {
	TemplateID     int64
	Destination    string
	Variables      map[string]json.RawMessage
	IdempotencyKey string
	Track          bool
}

// SendTransactional is the Transactional send in the Workspace of s (ADR 0005, ADR 0112): a
// single-recipient email rendered from a Template the request references, with per-call variables.
// It normalizes the destination, reads the Template in the Workspace, decodes the variables,
// resolves the Contact the address belongs to and sends the request through Send, which holds
// idempotency, the freeze check, the Sending-domain gate, eligibility, rendering, the provider call
// and the send record (ADR 0015). Its result is closed: a Transactional request is never rate
// limited or deferred (ADR 0023), so a Deferral that comes back is an ErrTransactionalInvariant.
func (m *Module) SendTransactional(ctx context.Context, s *ent.Scoped, req TransactionalRequest) (TransactionalResult, error) {
	dest := emailcase.Of(req.Destination)
	if dest == "" {
		return TransactionalResult{}, ErrTransactionalEmptyDestination
	}

	// Workspace-scoped: another Workspace's Template id is not found, never sent. The Template is
	// bound by reference (GLOSSARY "Transactional send"): its current content is what is sent.
	tmpl, err := s.EmailTemplate().Get(ctx, req.TemplateID)
	if ent.IsNotFound(err) {
		return TransactionalResult{}, ErrTransactionalTemplateNotFound
	}
	if err != nil {
		return TransactionalResult{}, fmt.Errorf("transactional send: read the template: %w", err)
	}

	vars, err := decodeVariables(req.Variables)
	if err != nil {
		return TransactionalResult{}, err
	}

	// The Contact this destination resolves to, when one exists (transactional mail may go to
	// an address with no Contact); the send fact attaches to it.
	contactID, err := contacts.ResolveID(ctx, s, "", &dest, nil)
	if err != nil {
		return TransactionalResult{}, err
	}

	// A client key makes the send idempotent; without one every call is its own send.
	res, err := m.Send(ctx, s, Transactional(TransactionalMessage{
		ClientKey:   req.IdempotencyKey,
		Destination: dest,
		ContactID:   contactID,
		TemplateID:  req.TemplateID,
		Content:     EmailContent{Subject: tmpl.Subject, Body: tmpl.Body},
		Variables:   vars,
		Track:       req.Track,
	}))
	if errors.Is(err, ErrInProgress) {
		return TransactionalResult{}, ErrTransactionalInProgress
	}
	if errors.Is(err, ErrKeyReused) {
		return TransactionalResult{}, ErrTransactionalKeyReused
	}
	if err != nil {
		return TransactionalResult{}, fmt.Errorf("transactional send: %w", err)
	}

	out := TransactionalResult{MessageID: res.MessageID, Destination: dest}
	switch res.Outcome {
	case Sent:
		out.Status = TransactionalSent
	case Skipped:
		// Transactional skips only on Suppression (the global hard floor).
		out.Status = TransactionalSuppressed
	case Failed:
		if res.Replayed {
			// A failed key is spent: the caller should retry under a fresh key.
			return TransactionalResult{}, ErrTransactionalReplayedFailed
		}
		return TransactionalResult{}, fmt.Errorf("%w: %s", emailrender.ErrInvalidContent, res.Reason)
	case Held:
		return TransactionalResult{}, ErrHeld.With(HoldDetail(res.Hold))
	default:
		// Deferral, and Refused (a Test send's outcome): a Transactional request is neither
		// reserved against the Provider limit nor a Test send.
		return TransactionalResult{}, fmt.Errorf("%w: outbound answered a transactional send with %q", ErrTransactionalInvariant, res.Outcome)
	}
	return out, nil
}

// decodeVariables turns the per-call variables into the binding map Liquid renders against.
func decodeVariables(raw map[string]json.RawMessage) (map[string]any, error) {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return nil, ErrTransactionalInvalidVariables.With(fmt.Sprintf("variable %q: %v", k, err))
		}
		out[k] = val
	}
	return out, nil
}
