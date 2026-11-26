// Package confirmationrequests is the Confirmation request (ADR 0013): one double opt-in
// confirmation email to a Contact's address, asked for by the Workspace (a Member, the API, MCP) or
// by the person (the page of an expired link). A request is decided now and its email is sent by a
// job through the Outbound send, so a Hold delays it as it delays a Broadcast and it never falls back
// to the System sender. Who asked decides what an opt-out of everything does: it refuses the
// Workspace's request and leaves the person's. Whoever asks, a Destination gets at most
// throttle.ConfirmationsPerDestinationDay confirmation emails from a Workspace in 24 hours.
package confirmationrequests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/riverqueue/river"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Status is what became of a Confirmation request that was not refused.
type Status string

const (
	// Accepted: the confirmation email is on its way.
	Accepted Status = "accepted"
	// Confirmed: the address already holds a Confirmation, and gets no email.
	Confirmed Status = "confirmed"
)

// The refusals of a request the Workspace makes. A Workspace with no confirmation email set up is
// workspaces.ErrConfirmationEmailRequired; one over the limit is a *ratelimit.WaitError.
var (
	// ErrContactNotFound: no such Contact in the Workspace.
	ErrContactNotFound = validation.NotFound("confirmation request: contact not found", i18n.ErrorsContactNotFound)
	// ErrNoEmail: the Contact has no email address to confirm.
	ErrNoEmail = validation.RefuseField(fieldname.Email, "confirmation request: the contact has no email", i18n.ErrorsContactEmailMissing)
	// ErrSuppressed: the address is suppressed, which no confirmation email passes.
	ErrSuppressed = validation.Conflict("confirmation request: the address is suppressed", i18n.ErrorsConfirmationRequestSuppressed)
	// ErrUnsubscribedEverything: the address opted out of everything, and only the person returns
	// (ADR 0121): soliciting re-permission from them is itself a violation.
	ErrUnsubscribedEverything = validation.Conflict("confirmation request: the address opted out of everything", i18n.ErrorsConfirmationRequestUnsubscribedEverything)
)

// Budget counts the confirmation emails of a Destination in a Workspace (ADR 0013, ADR 0025):
// TakeConfirmation reserves one and returns the zero Refusal when it was granted. *throttle.Attempts
// satisfies it.
type Budget interface {
	TakeConfirmation(ctx context.Context, workspaceID int64, destination string) (ratelimit.Refusal, error)
}

// Module decides Confirmation requests and sends their emails.
type Module struct {
	bus         *events.Bus
	budget      Budget
	out         *outboundmessages.Module
	eligibility *eligibility.Module
	now         func() time.Time
}

// New builds the module over the bus its requests queue their jobs through, the budget that limits
// them, the Outbound send their emails go through and the Instance clock.
func New(bus *events.Bus, budget Budget, out *outboundmessages.Module, now func() time.Time) *Module {
	return &Module{bus: bus, budget: budget, out: out, eligibility: eligibility.New(), now: now}
}

// AskByWorkspace is a Confirmation request a Member, the API or MCP makes for the Contact
// contactID. It is refused while the Workspace has no confirmation email set up, for a Contact with
// no address, and for an address that is suppressed or opted out of everything, naming which; one
// over the limit is a *ratelimit.WaitError. An address that holds a Confirmation gets nothing and is
// Confirmed. It works whether or not the Workspace requires confirmed opt-in.
func (m *Module) AskByWorkspace(ctx context.Context, s *ent.Scoped, contactID int64) (Status, error) {
	if err := m.setUp(ctx, s); err != nil {
		return "", err
	}
	c, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return "", ErrContactNotFound
	}
	if err != nil {
		return "", fmt.Errorf("confirmation request: read the contact: %w", err)
	}
	address := emailcase.Of(lo.FromPtr(c.Email))
	if address == "" {
		return "", ErrNoEmail
	}
	// A refusal comes first: a suppressed address that holds a Confirmation can receive nothing, which
	// the Workspace is told.
	dec, err := m.eligibility.Check(ctx, s, eligibility.CheckInput{Channel: channels.Email, Destination: address, Mode: eligibility.ConfirmationByWorkspace()})
	if err != nil {
		return "", err
	}
	switch dec.Reason {
	case skipreason.Suppressed:
		return "", ErrSuppressed
	case skipreason.UnsubscribedEverything:
		return "", ErrUnsubscribedEverything
	default:
		// Eligible: the Workspace's mode names no other layer.
	}
	confirmed, err := holdsConfirmation(ctx, s, address)
	if err != nil {
		return "", err
	}
	if confirmed {
		return Confirmed, nil
	}
	refusal, err := m.budget.TakeConfirmation(ctx, s.WorkspaceID(), address)
	if err != nil {
		return "", fmt.Errorf("confirmation request: take the limit: %w", err)
	}
	if refusal.Wait > 0 {
		return "", &ratelimit.WaitError{Policy: ratelimit.PolicyConfirmation, Refusal: refusal, At: m.now()}
	}
	return Accepted, m.enqueue(ctx, s, c.ID, outboundmessages.AskedByWorkspace)
}

// AskByPerson is a Confirmation request the person makes from the page of an expired link (ADR
// 0013), for the Destination and Contact the link names. Only a Suppression stops it. Its answer
// says nothing about an address: an address over its limit, suppressed, or no longer the Contact's
// (the Contact erased or its address changed) gets no email and the answer is Accepted all the same.
// An address that holds a Confirmation is Confirmed. It is refused only while the Workspace has no
// confirmation email set up.
func (m *Module) AskByPerson(ctx context.Context, s *ent.Scoped, link linktarget.Confirmation) (Status, error) {
	if err := m.setUp(ctx, s); err != nil {
		return "", err
	}
	address := emailcase.Of(link.Destination)
	confirmed, err := holdsConfirmation(ctx, s, address)
	if err != nil {
		return "", err
	}
	if confirmed {
		return Confirmed, nil
	}
	c, err := s.Contact().Get(ctx, link.ContactID)
	if ent.IsNotFound(err) {
		return Accepted, nil
	}
	if err != nil {
		return "", fmt.Errorf("confirmation request: read the contact: %w", err)
	}
	holds := emailcase.Of(lo.FromPtr(c.Email))
	if holds != address {
		return Accepted, nil
	}
	dec, err := m.eligibility.Check(ctx, s, eligibility.CheckInput{Channel: channels.Email, Destination: address, Mode: eligibility.ConfirmationByPerson()})
	if err != nil {
		return "", err
	}
	if !dec.Eligible {
		return Accepted, nil
	}
	refusal, err := m.budget.TakeConfirmation(ctx, s.WorkspaceID(), address)
	if err != nil {
		return "", fmt.Errorf("confirmation request: take the limit: %w", err)
	}
	if refusal.Wait > 0 {
		return Accepted, nil
	}
	return Accepted, m.enqueue(ctx, s, c.ID, outboundmessages.AskedByPerson)
}

// Page is what the confirmation page of a link shows (ADR 0013): whether the link has expired, and
// the Workspace that sent it, by its name and its Brand kit as it stands.
type Page struct {
	Expired       bool
	WorkspaceName string
	Brand         brandkits.Kit
}

// ErrLinkGone refuses the page of a link whose Workspace is gone: the link leads nowhere, as an
// invalid one does.
var ErrLinkGone = validation.LinkInvalid("confirmation request: the link's workspace is gone", i18n.ErrorsTrackingLinkInvalid)

// ReadPage reads what the confirmation page of a link of the Workspace of s shows; expired is the
// link's own. A Workspace erased since leaves nothing to show.
func (m *Module) ReadPage(ctx context.Context, s *ent.Scoped, expired bool) (Page, error) {
	ws, err := s.Workspace(ctx)
	if ent.IsNotFound(err) {
		return Page{}, ErrLinkGone
	}
	if err != nil {
		return Page{}, fmt.Errorf("confirmation page: read the workspace: %w", err)
	}
	state := workspaces.DeletionStateOf(ws)
	if state == workspaces.StateErased {
		return Page{}, ErrLinkGone
	}
	kit, err := brandkits.NewBrands().Kit(ctx, s)
	if err != nil {
		return Page{}, fmt.Errorf("confirmation page: read the brand kit: %w", err)
	}
	return Page{Expired: expired, WorkspaceName: ws.Name, Brand: kit}, nil
}

// setUp refuses a request while the Workspace has no confirmation email set up.
func (m *Module) setUp(ctx context.Context, s *ent.Scoped) error {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return fmt.Errorf("confirmation request: read the workspace: %w", err)
	}
	return workspaces.RequireConfirmationEmail(ws)
}

// holdsConfirmation reports whether the address holds a Confirmation, whatever its provenance.
func holdsConfirmation(ctx context.Context, s *ent.Scoped, address string) (bool, error) {
	ok, err := s.Confirmation().Query().Where(
		confirmation.ChannelEQ(channels.Email),
		confirmation.DestinationEQ(address),
	).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("confirmation request: read the confirmation: %w", err)
	}
	return ok, nil
}

// enqueue queues the request's email, under a key of its own.
func (m *Module) enqueue(ctx context.Context, s *ent.Scoped, contactID int64, by outboundmessages.ConfirmationAsker) error {
	return m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
		return q.Enqueue(ctx, SendArgs{
			WorkspaceID: ts.WorkspaceID(),
			ContactID:   contactID, AskedBy: by, Request: ulid.Make().String(),
		}, nil)
	})
}

// SendArgs is the job of one Confirmation request's email: the Contact, who asked, and the
// request's own id, which keys its Outbound message.
type SendArgs struct {
	jobkind.Workspace

	ContactID int64                              `json:"contact_id"`
	AskedBy   outboundmessages.ConfirmationAsker `json:"asked_by"`
	Request   string                             `json:"request"`
}

// Kind is the job's kind, which Erasure purges by its Contact.
func (SendArgs) Kind() string { return jobkind.SendConfirmation.Kind }

// Attempts is how many times a confirmation email is tried before it is given up.
const Attempts = 10

// InsertOpts gives the job its attempts.
func (SendArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: Attempts} }

// Outcome is what the job of a request's email does next: done (zero), wait Wait without spending
// an attempt, or retry Err.
type Outcome struct {
	Wait time.Duration
	Err  error
}

// inProgressWait is how long a job whose message another attempt holds waits before it looks again.
const inProgressWait = 30 * time.Second

// Attempt is the job's attempt at a request's email: its number and how many it has.
type Attempt struct {
	Number, Limit int
}

// last reports whether no attempt follows this one.
func (a Attempt) last() bool { return a.Number >= a.Limit }

// Deliver sends the email of a request, the Workspace's confirmation email as it stands now, to the
// Contact's address. A Contact gone since the request, or a confirmation email no longer set up,
// sends nothing. A Hold or a Deferral waits; any other failure is retried, and given up for good on
// the last attempt.
func (m *Module) Deliver(ctx context.Context, s *ent.Scoped, args SendArgs, attempt Attempt) Outcome {
	c, err := s.Contact().Get(ctx, args.ContactID)
	if ent.IsNotFound(err) {
		return Outcome{}
	}
	if err != nil {
		return Outcome{Err: fmt.Errorf("confirmation email: read the contact: %w", err)}
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Outcome{Err: fmt.Errorf("confirmation email: read the workspace: %w", err)}
	}
	if !workspaces.HasConfirmationEmail(ws) {
		slog.InfoContext(ctx, "confirmation email: no longer set up, nothing sent", "workspace_id", ws.ID)
		return Outcome{}
	}
	req := outboundmessages.Confirmation(outboundmessages.ConfirmationMessage{
		Request: args.Request, Contact: c, AskedBy: args.AskedBy, Email: workspaces.ConfirmationEmailOf(ws),
	})
	res, err := m.out.Send(ctx, s, req)
	switch {
	case errors.Is(err, outboundmessages.ErrInProgress):
		return Outcome{Wait: inProgressWait}
	case err != nil:
		if attempt.last() {
			err = errors.Join(err, m.out.MarkFailed(ctx, s, req.Key, err))
		}
		return Outcome{Err: err}
	}
	switch res.Outcome {
	case outboundmessages.Held:
		return Outcome{Wait: outboundmessages.HoldRetry().After}
	case outboundmessages.Deferral:
		return Outcome{Wait: res.Wait}
	case outboundmessages.Sent, outboundmessages.Skipped, outboundmessages.Failed,
		outboundmessages.Refused, outboundmessages.Withdrawn:
	}
	return Outcome{}
}
