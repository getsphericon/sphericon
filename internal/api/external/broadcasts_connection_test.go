package external_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// outsiderAddress is no User's: a Test send to it reaches someone the Connection does not act for.
const outsiderAddress = "outsider@example.com"

// A Test send through a Connection without mcp:send goes only to the User the Connection acts for
// (ADR 0132); with mcp:send, and through the Workspace's API token, it goes to any address.
func TestExternalTestSendGoesToTheAddressAskedFor(t *testing.T) {
	cases := []struct {
		name   string
		bearer func(env *testhelper.TestEnv) string
		to     string
	}{
		{"a connection to its user", func(env *testhelper.TestEnv) string {
			return env.ConnectionBearer(t, fixtures.OwnerJohnEmail, authorScopes...)
		}, "INFO@getsphericon.com"},
		{"a connection with mcp:send to another address", func(env *testhelper.TestEnv) string {
			return env.ConnectionBearer(t, fixtures.OwnerJohnEmail, append(authorScopes, apitokens.ScopeMcpSend)...)
		}, outsiderAddress},
		{"the workspace's api token to another address", func(env *testhelper.TestEnv) string {
			return env.ScopedBearer(t, authorScopes...)
		}, outsiderAddress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalWithToken(t, tc.bearer(env))

			err := c.BroadcastsTestSend(t.Context(), &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress(externalapi.EmailAddress(tc.to))},
				externalapi.BroadcastsTestSendParams{ID: draftBroadcast})

			require.NoError(t, err)
			messages := env.CustomerMail.Messages()
			require.Len(t, messages, 1)
			assert.Equal(t, strings.ToLower(tc.to), messages[0].To)
		})
	}
}

// A Connection without mcp:send cannot Test send to an address other than its User's: a 422 that
// says why, and nothing is sent (ADR 0132).
func TestExternalConnectionWithoutTheSendScopeTestSendsOnlyToItsUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalWithToken(t, env.ConnectionBearer(t, fixtures.OwnerJohnEmail, authorScopes...))

	err := c.BroadcastsTestSend(t.Context(), &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress(outsiderAddress)},
		externalapi.BroadcastsTestSendParams{ID: draftBroadcast})

	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "only to your own email address")
	assert.Empty(t, env.CustomerMail.Messages(), "nothing is sent")
}

// An SMS Broadcast has no Test send through a Connection without mcp:send: its User has no number
// the Connection may text (ADR 0132).
func TestExternalConnectionWithoutTheSendScopeHasNoSMSTestSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.AcmeTwilio(t)
	author := env.ExternalScoped(t, apitokens.ScopeBroadcastsWrite)
	made, err := author.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name: "Flash sale", Content: testhelper.ExternalSMS("Hi {{ first_name }}"),
		IntegrationId: externalapi.NewOptNilEntityId(entityIDString(int64(account.Integration.ID))),
		SegmentId:     externalapi.NilEntityId{Null: true},
	})
	require.NoError(t, err)
	c := env.ExternalWithToken(t, env.ConnectionBearer(t, fixtures.OwnerJohnEmail, authorScopes...))

	err = c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Phone: externalapi.NewOptString("+14155550199")},
		externalapi.BroadcastsTestSendParams{ID: made.ID})

	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "not to a phone")
	assert.Empty(t, env.Twilio.Messages(), "nothing is texted")
}
