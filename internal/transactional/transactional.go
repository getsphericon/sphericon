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
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/templates"
)

// Domain errors of a request that cannot be sent as asked.
var (
	// ErrEmptyDestination: the address is empty once normalized.
	ErrEmptyDestination = errors.New("transactional: destination must not be empty")
	// ErrTemplateNotFound: no such Template in the Workspace.
	ErrTemplateNotFound = errors.New("transactional: template not found")
	// ErrInvariant: Outbound answered with something a Transactional request can never get.
	// It is a bug, reported loudly, not a refusal the caller can act on.
	ErrInvariant = errors.New("transactional: invariant violated")
)

// InvalidVariablesError reports that a per-call variable is not valid JSON.
type InvalidVariablesError struct {
	Name string
	Err  error
}

func (e *InvalidVariablesError) Error() string {
	return fmt.Sprintf("variable %q: %v", e.Name, e.Err)
}

func (e *InvalidVariablesError) Unwrap() error { return e.Err }

// Status is how a Transactional request ended. It is closed: each value is a final answer or a
// reversible refusal, and a Deferral is not among them.
type Status string

const (
	// Sent: the provider accepted the message.
	Sent Status = "sent"
	// Suppressed: Suppression, the global hard floor, forbids the destination.
	Suppressed Status = "suppressed"
	// FailedRender: the Template cannot be rendered with these variables; Reason says why.
	FailedRender Status = "failed_render"
	// ReplayedFailed: the Idempotency-Key was spent by a send that failed; retry under a new key.
	ReplayedFailed Status = "replayed_failed"
	// InProgress: a send with this Idempotency-Key is still in flight.
	InProgress Status = "in_progress"
	// Held: a reversible hold on the Workspace or its Sending domain; Reason is the hold.
	Held Status = "held"
)

// Result is the answer to a Request.
type Result struct {
	Status Status
	// MessageID is the Outbound message of a Sent or Suppressed request.
	MessageID int64
	// Destination is the normalized address.
	Destination string
	// Reason explains FailedRender (the render error) and Held (a hold reason; see
	// outbound.Detail).
	Reason string
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
		return Result{Status: InProgress, Destination: dest}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("transactional send: %w", err)
	}

	out := Result{MessageID: res.MessageID, Destination: dest, Reason: res.Reason}
	switch res.Outcome {
	case outbound.Sent:
		out.Status = Sent
	case outbound.Skipped:
		// Transactional skips only on Suppression (the global hard floor).
		out.Status = Suppressed
	case outbound.Failed:
		out.Status = FailedRender
		if res.Replayed {
			// A failed key is spent: the caller should retry under a fresh key.
			out.Status = ReplayedFailed
		}
	case outbound.Held:
		out.Status = Held
	default:
		// Deferral, and Refused (a Test send's outcome): a Transactional request is neither
		// reserved against the Send rate limit nor a Test send.
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
			return nil, &InvalidVariablesError{Name: k, Err: err}
		}
		out[k] = val
	}
	return out, nil
}
