// Package transactional is the Transactional send operation (ADR 0005): a single-recipient
// email rendered from a referenced Template with per-call variables. It owns everything up to
// the Outbound send module: normalizing the destination, looking the Template up in the
// Workspace, decoding the variables, resolving the Contact the address belongs to and building
// the Outbound request. Idempotency, the freeze check, the Sending-domain gate, eligibility,
// rendering, the provider call and the send record stay the Outbound module's (ADR 0015).
//
// The module holds no database client: it takes the Workspace scope of the caller. Its Result
// is closed. A Deferral from Outbound is not one of its variants: a Transactional request is
// never rate limited or deferred (ADR 0023), so one that comes back is an ErrInvariant.
package transactional

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Domain errors of a request that cannot be sent as asked. A send Outbound holds is
// outbound.ErrHeld, and a Template that cannot render with these variables is
// emailrender.ErrInvalidContent, each with its reason.
var (
	// ErrEmptyDestination: the address is empty once normalized.
	ErrEmptyDestination = validation.Refuse("transactional: destination must not be empty", "errors.destination_empty")
	// ErrTemplateNotFound: no such Template in the Workspace.
	ErrTemplateNotFound = validation.NotFound("transactional: template not found", "errors.template_not_found")
	// ErrInvalidVariables: a per-call variable is not valid JSON; it is made With the variable
	// and why.
	ErrInvalidVariables = validation.Refuse("transactional: invalid variables", "errors.variables_invalid")
	// ErrInProgress: a send with this Idempotency-Key is still in flight.
	ErrInProgress = validation.Conflict("transactional: a send with this key is in progress", "errors.send_in_progress")
	// ErrReplayedFailed: the Idempotency-Key was spent by a send that failed; retry under a new
	// key.
	ErrReplayedFailed = validation.Conflict("transactional: a send with this key failed", "errors.send_key_failed")
	// ErrInvariant: Outbound answered with something a Transactional request can never get.
	// It is a bug, reported loudly, not a refusal the caller can act on.
	ErrInvariant = errors.New("transactional: invariant violated")
)

// Status is how a Transactional request that was not refused ended.
type Status string

const (
	// Sent: the provider accepted the message.
	Sent Status = "sent"
	// Suppressed: Suppression, the global hard floor, forbids the destination.
	Suppressed Status = "suppressed"
)

// Result is the answer to a Request.
type Result struct {
	Status Status
	// MessageID is the Outbound message of the request.
	MessageID int64
	// Destination is the normalized address.
	Destination string
}

// Request is one Transactional send. Variables are raw JSON values, decoded here to their
// natural Go types so merge tags like {{ amount }} render correctly.
type Request struct {
	TemplateID     int64
	Destination    string
	Variables      map[string]json.RawMessage
	IdempotencyKey string
	Track          bool
}

// Sender is the Outbound send module.
type Sender interface {
	Send(ctx context.Context, s *ent.Scoped, req outbound.Request) (outbound.Result, error)
}

// Module is the Transactional send operation.
type Module struct {
	templates *templates.Module
	sender    Sender
}

// New builds the module over the Templates module and the Outbound send module.
func New(t *templates.Module, sender Sender) *Module {
	return &Module{templates: t, sender: sender}
}

// Send runs the Transactional send in the Workspace of s.
func (m *Module) Send(ctx context.Context, s *ent.Scoped, req Request) (Result, error) {
	dest := suppression.NormalizeDestination(req.Destination)
	if dest == "" {
		return Result{}, ErrEmptyDestination
	}

	// Workspace-scoped: another Workspace's Template id is not found, never sent.
	tmpl, err := m.templates.Get(ctx, s, req.TemplateID)
	if errors.Is(err, templates.ErrNotFound) {
		return Result{}, ErrTemplateNotFound
	}
	if err != nil {
		return Result{}, err
	}

	vars, err := decodeVariables(req.Variables)
	if err != nil {
		return Result{}, err
	}

	// The Contact this destination resolves to, when one exists (transactional mail may go to
	// an address with no Contact); the send fact attaches to it.
	contactID, err := contacts.ResolveID(ctx, s, "", &dest, nil)
	if err != nil {
		return Result{}, err
	}

	// A client key makes the send idempotent; without one every call is its own send.
	res, err := m.sender.Send(ctx, s, outbound.Transactional(outbound.TransactionalMessage{
		ClientKey:   req.IdempotencyKey,
		Destination: dest,
		ContactID:   contactID,
		TemplateID:  req.TemplateID,
		Subject:     tmpl.Subject,
		Body:        tmpl.Body,
		Variables:   vars,
		Track:       req.Track,
	}))
	if errors.Is(err, outbound.ErrInProgress) {
		return Result{}, ErrInProgress
	}
	if err != nil {
		return Result{}, fmt.Errorf("transactional send: %w", err)
	}

	out := Result{MessageID: res.MessageID, Destination: dest}
	switch res.Outcome {
	case outbound.Sent:
		out.Status = Sent
	case outbound.Skipped:
		// Transactional skips only on Suppression (the global hard floor).
		out.Status = Suppressed
	case outbound.Failed:
		if res.Replayed {
			// A failed key is spent: the caller should retry under a fresh key.
			return Result{}, ErrReplayedFailed
		}
		return Result{}, fmt.Errorf("%w: %s", emailrender.ErrInvalidContent, res.Reason)
	case outbound.Held:
		return Result{}, outbound.ErrHeld.With(outbound.Detail(res.Reason))
	default:
		// Deferral, and Refused (a Test send's outcome): a Transactional request is neither
		// reserved against the Provider limit nor a Test send.
		return Result{}, fmt.Errorf("%w: outbound answered a transactional send with %q", ErrInvariant, res.Outcome)
	}
	return out, nil
}

// decodeVariables turns the per-call variables into the binding map Liquid renders against.
func decodeVariables(raw map[string]json.RawMessage) (map[string]any, error) {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return nil, ErrInvalidVariables.With(fmt.Sprintf("variable %q: %v", k, err))
		}
		out[k] = val
	}
	return out, nil
}
