package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalWebhooksShowTheFailureState(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "webhooks:read")

	got, err := c.WebhooksGet(context.Background(), externalapi.WebhooksGetParams{ID: entityIDString(fixtures.WebhookCodebasicsFailingID)})
	require.NoError(t, err)
	hook, ok := got.(*externalapi.WebhookResource)
	require.Truef(t, ok, "got %T", got)
	f, has := hook.Failure.Get()
	require.True(t, has)
	assert.Equal(t, externalapi.WebhookErrorKindHTTPStatus, f.Kind)
	assert.Equal(t, int32(503), f.Status.Value)
	assert.Equal(t, int32(7), f.ConsecutiveFailures)
}
