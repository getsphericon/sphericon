package outbound

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/validation"
)

// A Test send is email's (GLOSSARY): it goes through email's renderer and adapter, and the System
// sender stands in for the Integration only here.

// TestRequest is a user-initiated test send of a message to an address the
// author names explicitly (e.g. a Broadcast preview).
type TestRequest struct {
	To      string
	Content EmailContent
	// Variables are the sample merge tags the content is rendered with.
	Variables map[string]any
	// IntegrationID is the Integration the Test send goes through, as for a Request: nil is the
	// email default, and a named one whose Send route cannot send holds the Test send
	// (no_integration) without the System sender standing in.
	IntegrationID *int64
	// Asker is the User who asks. Set, it lets a Workspace with no verified Sending domain
	// send from the System sender, to Asker.Email only (ADR 0041). Nil (an API token) keeps
	// the Sending-domain gate for every Test send.
	Asker *Asker
}

// TestSubjectPrefix marks a preview send so it is never mistaken for the real mailing.
const TestSubjectPrefix = "[Test] "

// The refusals of a send a caller asked for and is told about at once (a Test send, a
// Transactional request), each carrying the reason worded by Detail.
var (
	// ErrHeld: the source cannot send right now (a Hold); it clears once the hold is lifted.
	ErrHeld = validation.Refuse("outbound: sending is held", i18n.ErrorsSendingHeld)
	// ErrTestRefused: the System sender will not make this Test send (ADR 0041).
	ErrTestRefused = validation.Refuse("outbound: the test send is refused", i18n.ErrorsTestSendRefused)
)

// SendBroadcastTest sends the Broadcast to one address with sample merge data for an author to
// preview it. A Held or Refused Test send is ErrHeld or ErrTestRefused made With the reason worded
// by Detail, content that does not render is emailrender.ErrInvalidContent with its reason, and a
// failed send is returned as the error it is, so every surface answers a refused preview the same
// way. The System sender's cap is no error: the Result is a Deferral whose Wait and Limit say when
// capacity returns.
func (m *Module) SendBroadcastTest(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, to string, asker *Asker) (Result, error) {
	content := BroadcastContent(b)
	content.Subject = TestSubjectPrefix + content.Subject
	res, err := m.SendTest(ctx, s, TestRequest{
		To:      to,
		Content: content,
		Variables: map[string]any{
			"first_name": "Alex", "last_name": "Sample", "email": to,
			"city": "Berlin", "country": "Germany", "locale": "de",
		},
		// The Integration a real send of the Broadcast would go through.
		IntegrationID: b.IntegrationID,
		Asker:         asker,
	})
	if err != nil {
		return Result{}, err
	}
	switch res.Outcome {
	case Held:
		return Result{}, ErrHeld.With(Detail(res.Reason))
	case Refused:
		return Result{}, ErrTestRefused.With(Detail(res.Reason))
	case Failed: // the content did not render
		return Result{}, fmt.Errorf("%w: %s", emailrender.ErrInvalidContent, res.Reason)
	case Sent, Skipped, Deferral:
		// Sent, or the System sender's cap, which Wait and Limit describe.
	}
	return res, nil
}

// SendTest sends one message for an author to preview. The author names the
// address, so it skips Send-eligibility and carries no unsubscribe or tracking, and
// it records no Outbound message and no email.sent Event — but it still passes the
// Workspace freeze and the Sending-domain gate and is DKIM-signed like any send: a
// suspended Workspace sends nothing, test or not (ADR 0015). Outcomes are Sent,
// Failed (the content did not render) or Held; there is no replay.
//
// The Test send goes through the Send route of the Integration the request names, else of the
// email default, as a real send does (#663). With an Asker, no verified Sending domain in
// the Workspace and that route either delivering outside or held for want of a default, it goes
// from the System sender instead (ADR 0041, ADR 0058): after the same freeze check, only to the
// Asker's own address, within the per-User and per-Workspace caps, and Refused when the Instance
// has no System sender. A route that keeps its mail (a Test inbox) takes the Test send itself and
// the Result is Captured; a named Integration that cannot send holds it (no_integration). The
// route is resolved once and decides both: no other module decides which sender a Test send uses.
func (m *Module) SendTest(ctx context.Context, s *ent.Scoped, req TestRequest) (Result, error) {
	ws, err := m.workspace(ctx, s)
	if err != nil {
		return Result{}, err
	}
	// The freeze holds a Test send whichever sender would make it.
	hold, err := m.holds.Reason(ctx, s)
	if err != nil {
		return Result{}, err
	}
	if hold != "" {
		return Result{Outcome: Held, Reason: hold}, nil
	}
	route, err := m.Route(ctx, s, messaging.ChannelEmail, req.IntegrationID)
	if err != nil {
		return Result{}, err
	}
	if req.Asker != nil {
		system, err := m.systemStandsIn(ctx, s, route, req.IntegrationID)
		if err != nil {
			return Result{}, err
		}
		if system {
			return m.sendTestFromSystem(ctx, s, ws, req)
		}
	}
	g, err := m.through(ctx, s, route, req.Content)
	if err != nil {
		return Result{}, err
	}
	if g.hold != "" {
		return Result{Outcome: Held, Reason: g.hold}, nil
	}
	adapter, ok := route.Sender.(emailAdapter)
	if !ok {
		return Result{}, fmt.Errorf("outbound: %T is no email adapter", route.Sender)
	}
	to := suppression.NormalizeDestination(req.To)
	msg, err := m.renderTest(ctx, s, ws, req, to)
	if err != nil {
		return failedContent(err)
	}
	res, err := sendTestMessage(ctx, adapter.sender, msg, g.as.address, g.as.name, to)
	res.Captured = res.Outcome == Sent && g.captured()
	return res, err
}

// systemStandsIn reports whether the System sender makes an asked-for Test send in place of the
// route (ADR 0041, ADR 0058): only while none of the Workspace's Sending domains is verified, and
// only where the route would deliver outside or the Workspace has no default to send through. A
// route that keeps its mail takes the Test send itself, so it is captured; a named Integration that
// cannot send is no reason for the System sender, and the gate holds the Test send as it holds a
// real one.
func (m *Module) systemStandsIn(ctx context.Context, s *ent.Scoped, route Route, integrationID *int64) (bool, error) {
	anyVerified, err := s.SendingDomain().Query().Where(dkimstate.Verified()).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("outbound: check sending domains: %w", err)
	}
	if anyVerified {
		return false, nil
	}
	if route.Held() {
		return integrationID == nil, nil
	}
	return route.DeliversOutside, nil
}

// failedContent is a Test send whose render failed: a content error is the Failed result, any other
// is returned as the error it is.
func failedContent(err error) (Result, error) {
	if isContentError(err) {
		return Result{Outcome: Failed, Reason: err.Error()}, nil
	}
	return Result{}, err
}

// renderTest renders a Test send's content for to with email's renderer: no unsubscribe, no
// tracking.
func (m *Module) renderTest(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, req TestRequest, to string) (messaging.EmailMessage, error) {
	return m.renderEmail(ctx, s, Request{Content: req.Content, Variables: req.Variables}, 0, to, ws)
}

// sendTestMessage sends a rendered Test message from the address from and reports it Sent. No
// Outbound message is recorded, and the message carries no Origin: it is a Test send's.
func sendTestMessage(ctx context.Context, sender messaging.EmailSender, msg messaging.EmailMessage, from, name, to string) (Result, error) {
	msg.From, msg.FromName, msg.To = from, name, to
	if _, err := sendEmail(ctx, sender, msg); err != nil {
		if reason := heldBy(err); reason != "" {
			return Result{Outcome: Held, Reason: reason}, nil
		}
		return Result{}, fmt.Errorf("outbound: test send to %s: %w", to, err)
	}
	return Result{Outcome: Sent, From: from}, nil
}

// sendTestFromSystem is the Test send of a Workspace with no verified Sending domain, past the
// Workspace freeze.
func (m *Module) sendTestFromSystem(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, req TestRequest) (Result, error) {
	to := suppression.NormalizeDestination(req.To)
	if to != suppression.NormalizeDestination(req.Asker.Email) {
		return Result{Outcome: Refused, Reason: RefuseOtherRecipient}, nil
	}
	if m.system == nil {
		return Result{Outcome: Refused, Reason: RefuseNoSystemSender}, nil
	}
	// Render before taking a slot: content that cannot render never costs the budget.
	msg, err := m.renderTest(ctx, s, ws, req, to)
	if err != nil {
		return failedContent(err)
	}
	// A System sender that names no From cannot send: refuse before a slot is taken.
	from, name := effectiveFrom(m.system, "", "")
	if from == "" {
		return Result{Outcome: Refused, Reason: RefuseNoSystemSender}, nil
	}
	if m.testBudget != nil {
		refusal, err := m.testBudget.TakeTestSend(ctx, req.Asker.UserID, s.WorkspaceID())
		if err != nil {
			return Result{}, fmt.Errorf("outbound: test send budget: %w", err)
		}
		if refusal.Wait > 0 {
			return Result{Outcome: Deferral, Refusal: refusal}, nil
		}
	}
	return sendTestMessage(ctx, m.system, msg, from, name, to)
}
