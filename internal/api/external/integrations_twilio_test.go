package external_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const twilioNumber = "+15017250604"

func externalTwilio(account twiliotest.Account, key twiliotest.APIKey, secret string) externalapi.IntegrationConfigInput {
	return externalapi.IntegrationConfigInput{OneOf: externalapi.NewTwilioConfigInputIntegrationConfigInputSum(externalapi.TwilioConfigInput{
		Kind: externalapi.TwilioConfigInputKindTwilio, AccountSid: account.SID,
		ApiKeySid: externalapi.NewOptNilString(key.SID), Secret: secret, From: twilioNumber,
	})}
}

func authToken(token string) externalapi.OptJsonObject {
	return externalapi.NewOptJsonObject(externalapi.JsonObject{"auth_token": []byte(`"` + token + `"`)})
}

// An API client connects, reads, rotates and archives a Twilio account as the app does (ADR 0016).
func TestExternalTwilioIntegrationLifecycle(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	account := env.Twilio.AddAccount()
	key := env.Twilio.AddAPIKey(account)
	number := env.Twilio.AddNumber(account, twilioNumber)

	t.Run("a bad key is refused", func(t *testing.T) {
		res, err := c.IntegrationsCreate(t.Context(), &externalapi.CreateIntegrationInput{
			Name: "SMS", Config: externalTwilio(account, key, "wrongsecret"), Params: authToken(account.AuthToken),
		})
		require.NoError(t, err)
		refused, ok := res.(*externalapi.IntegrationsCreateUnprocessableEntity)
		require.Truef(t, ok, "got %T", res)
		assert.Contains(t, refused.Errors.Value, "config.secret")
	})

	res, err := c.IntegrationsCreate(t.Context(), &externalapi.CreateIntegrationInput{
		Name: "SMS", Config: externalTwilio(account, key, key.Secret), Params: authToken(account.AuthToken),
		MaxPerSecond: externalapi.NewOptNilMaxPerSecond(10),
	})
	require.NoError(t, err)
	row, ok := res.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, externalapi.IntegrationChannelSMS, row.Channel)
	assert.Equal(t, externalapi.IntegrationProviderTwilio, row.Provider)
	assert.Equal(t, row.HookUrl, env.Twilio.Webhooks(number).Inbound)
	assert.Equal(t, externalapi.NewNilInt32(10), row.ProviderLimit.PerSecond.Limit, "the Provider limit is the user's")

	got, err := c.IntegrationsGet(t.Context(), externalapi.IntegrationsGetParams{ID: row.ID})
	require.NoError(t, err)
	read, ok := got.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", got)
	cfg, ok := read.Config.OneOf.GetTwilioConfig()
	require.True(t, ok)
	assert.Equal(t, account.SID, cfg.AccountSid)
	assert.Equal(t, key.SID, cfg.ApiKeySid.Value)
	assert.Equal(t, externalapi.SmsFromKindNumber, cfg.FromKind)
	assert.JSONEq(t, `"********"`, string(read.Params["auth_token"]))
	assert.NotContains(t, readBody(t, read), key.Secret)
	assert.NotContains(t, readBody(t, read), account.AuthToken)

	upd, err := c.IntegrationsUpdate(t.Context(), &externalapi.UpdateIntegrationInput{Params: authToken("rotatedauthtoken")},
		externalapi.IntegrationsUpdateParams{ID: row.ID})
	require.NoError(t, err)
	rotated, ok := upd.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", upd)
	assert.Equal(t, []string{"auth_token"}, rotated.SecretsRetiring)
	assert.Equal(t, read.Config, rotated.Config, "the send credential stays")

	out, err := c.IntegrationsDelete(t.Context(), externalapi.IntegrationsDeleteParams{ID: row.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteNoContent{}, out)
}

// readBody is the resource as the API writes it.
func readBody(t *testing.T, r *externalapi.IntegrationResource) string {
	t.Helper()
	b, err := r.MarshalJSON()
	require.NoError(t, err)
	return string(b)
}
