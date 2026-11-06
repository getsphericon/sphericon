package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outbound"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A template that cannot render is a 422 naming the cause; the key is then spent,
// so retrying it replays the failure as a 409 instead of sending.
func TestExternalEmailsSendFailedRenderSpendsTheIdempotencyKey(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "emails:send")

	send := func() error {
		_, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
			TemplateId: fixtures.TemplateBrokenSubjectID, Destination: "render@example.com",
		}, externalapi.EmailsSendParams{IdempotencyKey: externalapi.NewOptString("render-1")})
		return err
	}

	first := testhelper.ExternalRefusal(t, send(), http.StatusUnprocessableEntity)
	assert.Contains(t, first.Detail.Value, "message content cannot be rendered")

	rec, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.WorkspaceID(fixtures.AcmeID), outboundmessage.Destination("render@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, outboundmessage.StatusFailed, rec.Status)

	testhelper.ExternalRefusal(t, send(), http.StatusConflict, "a failed key is spent")
	assert.Empty(t, env.CustomerMail.Messages())
}

// A suspended workspace sends nothing: the hold is a 422 with the hold's wording
// and records no message, so the request can succeed once the freeze is lifted.
func TestExternalEmailsSendIsHeldWhileTheWorkspaceIsSuspended(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "emails:send")
	tmpl := variablesTemplate(env.DB)
	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())

	_, err := c.EmailsSend(ctx, &externalapi.SendTransactionalEmailInput{
		TemplateId: templateID(tmpl), Destination: "held@example.com",
	}, externalapi.EmailsSendParams{})
	held := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, "the message was not sent: "+outbound.Detail(holds.ReasonSuspended), held.Detail.Value)
	assert.Empty(t, env.CustomerMail.Messages())
	n, err := env.DB.OutboundMessage.Query().Where(outboundmessage.Destination("held@example.com")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a hold records nothing")
}
