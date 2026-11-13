package outboundmessages

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// A Test send (GLOSSARY) goes to an address the author names, through the Channel's renderer and
// adapter: on email to an address, where the System sender stands in for the Integration only
// here; on SMS to a phone number, through the Broadcast's Integration (ADR 0080).

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
	// ErrNoTestSend: the Broadcast's Channel has no Test send. Telegram has none: the author would
	// need a Binding of their own (ADR 0080).
	ErrNoTestSend = validation.Refuse("outbound: the channel has no test send", i18n.ErrorsBroadcastNoTestSend)
	// ErrTestEmailRequired: a Test send of an email Broadcast names no address.
	ErrTestEmailRequired = validation.RefuseField(fieldTestEmail, "outbound: the test send names no email address", i18n.ErrorsTestSendEmailRequired)
	// ErrTestPhoneInvalid: a Test send of an SMS Broadcast names no phone number in international
	// format: one with its country code, which E.164 is written from.
	ErrTestPhoneInvalid = validation.RefuseField(fieldTestPhone, "outbound: the test number is not a phone number in international format", i18n.ErrorsPhoneInvalid)
)

// The request fields a Test send names its address in, one per Channel that has a Test send.
const (
	fieldTestEmail = "email"
	fieldTestPhone = "phone"
)

// TestAddress is where the author asks a Test send of a Broadcast to go: Email on an email
// Broadcast, Phone on an SMS one. The field of the other Channel is not read.
type TestAddress struct {
	Email string
	Phone string
}

// SendBroadcastTest sends the Broadcast to one address with sample merge data for an author to
// preview it: an email Broadcast to to.Email (ErrTestEmailRequired without one), an SMS Broadcast
// to to.Phone (ErrTestPhoneInvalid unless it is a number with its country code). A Broadcast of a
// Channel with no Test send is ErrNoTestSend. A Held or Refused Test send is ErrHeld or
// ErrTestRefused made With the reason worded by Detail, content that does not render is
// emailrender.ErrInvalidContent with its reason, and a failed send is returned as the error it is,
// so every surface answers a refused preview the same way. Who asks is the scope's actor, as for
// SendTest, and the System sender's cap is its *ratelimit.WaitError.
func (m *Module) SendBroadcastTest(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, to TestAddress) (Result, error) {
	stored, err := BroadcastContent(b)
	if err != nil {
		return Result{}, err
	}
	switch content := stored.(type) {
	case EmailContent:
		return m.sendEmailBroadcastTest(ctx, s, b, content, to.Email)
	case SMSContent:
		return m.sendSMSTest(ctx, s, content, to.Phone, b.IntegrationID)
	default:
		return Result{}, ErrNoTestSend
	}
}

// testSampleEmail is the sample email merge value of a Test send that goes to no address.
const testSampleEmail = "alex@example.com"

// testVariables are the sample merge data a Test send is rendered with.
func testVariables(email string) map[string]any {
	return map[string]any{
		"first_name": "Alex", "last_name": "Sample", "email": email,
		"city": "Berlin", "country": "Germany", "locale": "de",
	}
}

// sendEmailBroadcastTest is the Test send of an email Broadcast to the address to.
func (m *Module) sendEmailBroadcastTest(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, content EmailContent, to string) (Result, error) {
	if to == "" {
		return Result{}, ErrTestEmailRequired
	}
	content.Subject = TestSubjectPrefix + content.Subject
	res, err := m.SendTest(ctx, s, TestRequest{
		To:        to,
		Content:   content,
		Variables: testVariables(to),
		// The Integration a real send of the Broadcast would go through.
		IntegrationID: b.IntegrationID,
	})
	if err != nil {
		return Result{}, err
	}
	switch res.Outcome {
	case Held:
		return Result{}, ErrHeld.With(HoldDetail(res.Hold))
	case Refused:
		return Result{}, ErrTestRefused.With(RefuseDetail(res.Reason))
	case Failed: // the content did not render
		return Result{}, fmt.Errorf("%w: %s", emailrender.ErrInvalidContent, res.Reason)
	case Sent, Skipped, Deferral:
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
// email default, as a real send does (#663). Asked for by a User (the scope's actor: a /site
// session, never a token), with no verified Sending domain in the Workspace and that route either
// delivering outside or held for want of a default, it goes from the System sender instead
// (ADR 0041, ADR 0058): after the same freeze check, only to the User's own address, and Refused
// when the Instance has no System sender. Past the per-User or the per-Workspace cap it is not
// made: the error is a *ratelimit.WaitError that says when a slot frees. A route that keeps its mail (a Test inbox) takes the Test send itself and
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
		return Result{Outcome: Held, Hold: hold}, nil
	}
	route, err := m.Route(ctx, s, channels.Email, req.IntegrationID)
	if err != nil {
		return Result{}, err
	}
	if userID, ok := askingUser(s); ok {
		system, err := m.systemStandsIn(ctx, s, route, req.IntegrationID)
		if err != nil {
			return Result{}, err
		}
		if system {
			return m.sendTestFromSystem(ctx, s, ws, req, userID)
		}
	}
	g, err := m.through(ctx, s, route, req.Content)
	if err != nil {
		return Result{}, err
	}
	if g.hold != "" {
		return Result{Outcome: Held, Hold: g.hold}, nil
	}
	adapter, ok := route.Sender.(emailAdapter)
	if !ok {
		return Result{}, fmt.Errorf("outbound: %T is no email adapter", route.Sender)
	}
	to := emailcase.Of(req.To)
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
			return Result{Outcome: Held, Hold: reason}, nil
		}
		return Result{}, fmt.Errorf("outbound: test send to %s: %w", to, err)
	}
	return Result{Outcome: Sent, From: from}, nil
}

// askingUser is the User a Test send is asked for by: the scope's actor when it is a User. Any other
// actor (an API token, MCP's Connection) is no User a Test send from the System sender may go to.
func askingUser(s *ent.Scoped) (int64, bool) {
	actor := s.Actor()
	if actor.Kind != events.ActorUser {
		return 0, false
	}
	id, err := publicid.Decode(actor.ID)
	return id, err == nil
}

// errNoUsers is a module asked for a Test send by a User without the Users to find them among: a
// configuration error (WithUsers), never a silent refusal.
var errNoUsers = errors.New("outbound: a Test send asked by a User needs the Users (WithUsers)")

// sendTestFromSystem is the Test send of a Workspace with no verified Sending domain, past the
// Workspace freeze, asked for by the User userID.
func (m *Module) sendTestFromSystem(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, req TestRequest, userID int64) (Result, error) {
	if m.users == nil {
		return Result{}, errNoUsers
	}
	asker, err := m.users.User(ctx, userID)
	if err != nil {
		return Result{}, fmt.Errorf("outbound: the user who asks for a test send: %w", err)
	}
	to := emailcase.Of(req.To)
	if to != emailcase.Of(asker.Email) {
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
	from, name := messaging.EffectiveFrom(m.system, "", "")
	if from == "" {
		return Result{Outcome: Refused, Reason: RefuseNoSystemSender}, nil
	}
	if m.testBudget != nil {
		refusal, err := m.testBudget.TakeTestSend(ctx, userID, s.WorkspaceID())
		if err != nil {
			return Result{}, fmt.Errorf("outbound: test send budget: %w", err)
		}
		if refusal.Wait > 0 {
			return Result{}, &ratelimit.WaitError{Policy: ratelimit.PolicyTestSend, Refusal: refusal, At: m.now()}
		}
	}
	return sendTestMessage(ctx, m.system, msg, from, name, to)
}
