package external_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/suppression"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mjmlBody = `<mjml><mj-body><mj-section><mj-column><mj-text>Hi {{ name }}, your code is {{ code }}</mj-text></mj-column></mj-section></mj-body></mjml>`

func seedTemplate(t *testing.T, db *ent.Client, workspaceID int64) *ent.EmailTemplate {
	t.Helper()
	tmpl, err := db.EmailTemplate.Create().
		SetWorkspaceID(workspaceID).
		SetName("Receipt").
		SetSubject("Hello {{ name }}").
		SetBody(mjmlBody).
		Save(context.Background())
	require.NoError(t, err)
	return tmpl
}

func templateID(tmpl *ent.EmailTemplate) externalapi.EntityId {
	return externalapi.EntityId(strconv.FormatInt(tmpl.ID, 10))
}

// A transactional send renders the referenced template with per-call variables
// and is accepted by the (capturing) provider.
func TestExternalEmailsSend(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "Customer@Example.com",
		Variables: externalapi.NewOptSendTransactionalEmailInputVariables(
			externalapi.SendTransactionalEmailInputVariables{
				"name": jx.Raw(`"Ada"`),
				"code": jx.Raw(`123456`),
			}),
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.SendTransactionalEmailResponse)
	require.Truef(t, isOK, "got %T", res)
	assert.Equal(t, externalapi.TransactionalSendStatusSent, ok.Status)
	assert.Equal(t, "customer@example.com", ok.Destination, "destination normalized")
	assert.NotEmpty(t, ok.ID, "the durable send record id is returned")

	// The capturing provider received the rendered message — template content is
	// referenced and rendered at send time, variables merged, no raw braces.
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "customer@example.com", msgs[0].To)
	assert.Equal(t, "Hello Ada", msgs[0].Subject)
	assert.Contains(t, msgs[0].HTML, "Ada")
	assert.Contains(t, msgs[0].HTML, "123456")
	assert.NotContains(t, msgs[0].HTML, "{{")
}

// The verified-domain send gate (ADR 0010 slice 3): when the send path rejects an
// unverified From domain, the transactional API returns 422 (a client-correctable
// condition) rather than a 500. It is a reversible hold (ADR 0015), not a failed
// send: nothing is recorded, so the same request can succeed once the domain is back.
func TestExternalEmailsSendRejectsUnverifiedDomain(t *testing.T) {
	env := testhelper.Setup(t)
	env.CustomerMail.SetErr(messaging.ErrUnverifiedSendingDomain)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "customer@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	_, isUnprocessable := res.(*externalapi.EmailsSendUnprocessableEntity)
	require.Truef(t, isUnprocessable, "got %T, want 422 unprocessable", res)

	n, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.DestinationEQ("customer@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a hold records no message")
}

// A suppressed destination is skipped (Suppression is the hard floor on every
// surface) and reported as status "suppressed" — not an error.
func TestExternalEmailsSendSuppressed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	_, err := env.DB.Suppression.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel(suppression.ChannelEmail).
		SetDestination("blocked@example.com").
		SetReason(suppression.ReasonBounce).Save(ctx)
	require.NoError(t, err)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "blocked@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.SendTransactionalEmailResponse)
	require.Truef(t, isOK, "got %T", res)
	assert.Equal(t, externalapi.TransactionalSendStatusSuppressed, ok.Status)
	assert.Empty(t, env.CustomerMail.Messages(), "suppressed destination is not sent to")
}

// Transactional skips Unsubscribe: an "everything" opt-out does NOT block a
// transactional send (you cannot unsubscribe from your own password reset).
func TestExternalEmailsSendIgnoresUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	_, err := env.DB.Unsubscribe.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetChannel("email").
		SetDestination("optout@example.com").
		SetSendingSource(eligibility.SourceEverything).Save(ctx)
	require.NoError(t, err)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "optout@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	ok := res.(*externalapi.SendTransactionalEmailResponse)
	assert.Equal(t, externalapi.TransactionalSendStatusSent, ok.Status)
	assert.Len(t, env.CustomerMail.Messages(), 1)
}

// Multi-tenant isolation: a token cannot send with another workspace's template.
func TestExternalEmailsSendCrossWorkspaceTemplate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	otherTmpl := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateGlobexID)

	// Token belongs to workspace 1; referencing ws2's template id must 404.
	c := env.ExternalScoped(t, "emails:send")
	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(otherTmpl),
		Destination: "x@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailsSendNotFound{}, res)
	assert.Empty(t, env.CustomerMail.Messages())
}

func TestExternalEmailsSendMissingTemplate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")

	res, err := c.EmailsSend(context.Background(), &externalapi.SendTransactionalEmailInput{
		TemplateId:  "999999",
		Destination: "x@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailsSendNotFound{}, res)
}

// emails:send is required.
func TestExternalEmailsSendScope(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	res, err := c.EmailsSend(context.Background(), &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "x@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailsSendUnauthorized{}, res)
}

// A successful send writes a durable record (the trace), resolves the destination
// to a contact when one exists, and publishes email.sent onto the outbox so the
// send is segmentable like broadcast/automation sends.
func TestExternalEmailsSendRecordsTrace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	contact, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("trace@example.com").Save(ctx)
	require.NoError(t, err)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "trace@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	require.IsType(t, &externalapi.SendTransactionalEmailResponse{}, res)

	rec, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.Destination("trace@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, outboundmessage.StatusSent, rec.Status)
	assert.Equal(t, outboundmessage.KindTransactional, rec.Kind)
	assert.Equal(t, "trace@example.com", rec.Destination)
	require.NotNil(t, rec.TemplateID)
	assert.Equal(t, tmpl.ID, *rec.TemplateID)
	require.NotNil(t, rec.ContactID)
	assert.Equal(t, contact.ID, *rec.ContactID, "destination resolved to the contact")

	sent := env.OutboxCount(t, "email.sent", map[string]any{"email": "trace@example.com"})
	assert.Equal(t, 1, sent, "email.sent published once")
}

// A repeated Idempotency-Key replays the original outcome without a second send.
func TestExternalEmailsSendIdempotentReplay(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	send := func() *externalapi.SendTransactionalEmailResponse {
		res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
			TemplateId:  templateID(tmpl),
			Destination: "once@example.com",
		}, externalapi.EmailsSendParams{IdempotencyKey: externalapi.NewOptString("key-1")})
		require.NoError(t, err)
		ok, isOK := res.(*externalapi.SendTransactionalEmailResponse)
		require.Truef(t, isOK, "got %T", res)
		return ok
	}

	first := send()
	second := send()

	assert.Equal(t, first.ID, second.ID, "same key replays the same record")
	assert.Equal(t, externalapi.TransactionalSendStatusSent, second.Status)
	assert.Len(t, env.CustomerMail.Messages(), 1, "the email is sent exactly once")

	n, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.Destination("once@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "one record for the key")
}

// A suppressed send still records a row and honors the idempotency key, but emits
// no email and no email.sent event.
func TestExternalEmailsSendSuppressedRecordsNoEvent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	_, err := env.DB.Suppression.Create().
		SetWorkspaceID(fixtures.AcmeID).SetChannel(suppression.ChannelEmail).
		SetDestination("blocked@example.com").SetReason(suppression.ReasonBounce).Save(ctx)
	require.NoError(t, err)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "blocked@example.com",
	}, externalapi.EmailsSendParams{IdempotencyKey: externalapi.NewOptString("sup-1")})
	require.NoError(t, err)
	ok := res.(*externalapi.SendTransactionalEmailResponse)
	assert.Equal(t, externalapi.TransactionalSendStatusSuppressed, ok.Status)

	rec, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.Destination("blocked@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, outboundmessage.StatusSkipped, rec.Status)

	events := env.OutboxCount(t, "email.sent", map[string]any{"email": "blocked@example.com"})
	assert.Zero(t, events, "no send happened, so no email.sent")
}

// An in-flight key (a pending record) returns 409 rather than sending again.
func TestExternalEmailsSendConflictOnPending(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")
	ctx := context.Background()
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)

	// Simulate a concurrent request that already claimed the key and is still sending.
	_, err := env.DB.OutboundMessage.Create().
		SetWorkspaceID(fixtures.AcmeID).SetKind(outboundmessage.KindTransactional).
		SetDestination("busy@example.com").SetTemplateID(tmpl.ID).
		SetStatus(outboundmessage.StatusPending).SetIdempotencyKey("transactional:inflight").Save(ctx)
	require.NoError(t, err)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId:  templateID(tmpl),
		Destination: "busy@example.com",
	}, externalapi.EmailsSendParams{IdempotencyKey: externalapi.NewOptString("inflight")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailsSendConflict{}, res)
	assert.Empty(t, env.CustomerMail.Messages())
}
