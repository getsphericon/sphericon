package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ent/outboundmessage"
	externalapi "github.com/mokevnin/sphericon/gen/external"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/outbound"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

// A template id that is not an int64 can name no template: 404, nothing sent.
func TestExternalEmailsSendWithAnUnparsableTemplateIDIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "emails:send")

	res, err := c.EmailsSend(context.Background(), &externalapi.SendTransactionalEmailInput{
		TemplateId: overflowID, Destination: "a@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailsSendNotFound{}, res)
	assert.Empty(t, env.CustomerMail.Messages())
}

// A template that cannot render is a 422 naming the cause; the key is then spent,
// so retrying it replays the failure as a 409 instead of sending.
func TestExternalEmailsSendFailedRenderSpendsTheIdempotencyKey(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "emails:send")
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)
	tmpl = env.DB.EmailTemplate.UpdateOne(tmpl).SetSubject("{% if %}broken").SaveX(ctx)

	send := func() externalapi.EmailsSendRes {
		res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
			TemplateId: templateID(tmpl), Destination: "render@example.com",
		}, externalapi.EmailsSendParams{IdempotencyKey: externalapi.NewOptString("render-1")})
		require.NoError(t, err)
		return res
	}

	first, ok := send().(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok)
	assert.Contains(t, first.Detail.Value, "render template")

	rec, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.Destination("render@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, outboundmessage.StatusFailed, rec.Status)

	assert.IsType(t, &externalapi.EmailsSendConflict{}, send(), "a failed key is spent")
	assert.Empty(t, env.CustomerMail.Messages())
}

// A suspended workspace sends nothing: the hold is a 422 with the hold's wording
// and records no message, so the request can succeed once the freeze is lifted.
func TestExternalEmailsSendIsHeldWhileTheWorkspaceIsSuspended(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "emails:send")
	tmpl := seedTemplate(t, env.DB, fixtures.AcmeID)
	suspended, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.Operator("op-42"), "abuse report")
	require.NoError(t, err)
	require.True(t, suspended)

	res, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId: templateID(tmpl), Destination: "held@example.com",
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	held, ok := res.(*externalapi.EmailsSendUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, outbound.HoldDetail(outbound.HoldSuspended), held.Detail.Value)
	assert.Empty(t, env.CustomerMail.Messages())
	n, err := env.DB.OutboundMessage.Query().Where(outboundmessage.Destination("held@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a hold records nothing")
}
