package site_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func createTwilio(t *testing.T, c *siteapi.Client, cfg siteapi.IntegrationConfigInput, authToken string) siteapi.SiteIntegrationsCreateRes {
	t.Helper()
	in := &siteapi.CreateIntegrationInput{Name: "Acme SMS", Config: cfg}
	if authToken != "" {
		in.Params = siteapi.NewOptJsonObject(testhelper.TwilioAuthToken(authToken))
	}
	res, err := c.SiteIntegrationsCreate(t.Context(), in, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return res
}

func twilioCreated(t *testing.T, res siteapi.SiteIntegrationsCreateRes) *siteapi.SiteIntegrationResource {
	t.Helper()
	row, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", res)
	return row
}

func twilioConfigOf(t *testing.T, row *siteapi.SiteIntegrationResource) siteapi.SiteTwilioConfig {
	t.Helper()
	cfg, ok := row.Config.OneOf.GetSiteTwilioConfig()
	require.Truef(t, ok, "config is twilio, got %v", row.Config.OneOf.Type)
	return cfg
}

func TestSiteTwilioConnectsANumberWithAnAPIKey(t *testing.T) {
	env := testhelper.Setup(t)

	sms := env.ConnectTwilio(t)

	row := sms.Integration
	assert.Equal(t, siteapi.SiteIntegrationChannelSMS, row.Channel)
	assert.Equal(t, siteapi.SiteIntegrationProviderTwilio, row.Provider)
	assert.Equal(t, "twilio", row.Preset.Or(""))
	cfg := twilioConfigOf(t, row)
	assert.Equal(t, sms.Account.SID, cfg.AccountSid)
	assert.Equal(t, sms.Key.SID, cfg.ApiKeySid.Value)
	assert.Equal(t, testhelper.TwilioNumber, cfg.From)
	assert.Equal(t, siteapi.SmsFromKindNumber, cfg.FromKind)
	assert.True(t, cfg.CallbacksSet)
	assert.JSONEq(t, `"********"`, string(row.Params["auth_token"]), "the webhook secret is never shown")
	assert.True(t, row.ProviderLimit.PerSecond.Limit.Null, "no Provider limit is read from Twilio")
	hooks := env.Twilio.Webhooks(sms.NumberSID)
	assert.Equal(t, row.HookUrl, hooks.Inbound, "the number's inbound messages reach the hook")
}

func TestSiteTwilioPointsAMessagingServiceAtTheHook(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	account := env.Twilio.AddAccount()
	service := env.Twilio.AddMessagingService(account)

	row := twilioCreated(t, createTwilio(t, c, testhelper.SiteTwilio(account.SID, "", account.AuthToken, service), account.AuthToken))

	cfg := twilioConfigOf(t, row)
	assert.Equal(t, siteapi.SmsFromKindMessagingService, cfg.FromKind)
	assert.True(t, cfg.ApiKeySid.Null, "sent with the Auth Token")
	assert.True(t, cfg.CallbacksSet)
	hooks := env.Twilio.Webhooks(service)
	assert.Equal(t, row.HookUrl, hooks.Inbound)
	assert.Equal(t, row.HookUrl, hooks.Status)
	assert.Equal(t, "false", hooks.UseInboundWebhookOnNumber, "the service's webhook is the one Twilio calls")
}

func TestSiteTwilioAnAlphanumericSenderLeavesTheAddressToCopy(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	account := env.Twilio.AddAccount()
	number := env.Twilio.AddNumber(account, testhelper.TwilioNumber)
	service := env.Twilio.AddMessagingService(account)

	row := twilioCreated(t, createTwilio(t, c, testhelper.SiteTwilio(account.SID, "", account.AuthToken, "Acme Shop"), account.AuthToken))

	cfg := twilioConfigOf(t, row)
	assert.Equal(t, siteapi.SmsFromKindAlphanumeric, cfg.FromKind)
	assert.False(t, cfg.CallbacksSet)
	numberHooks := env.Twilio.Webhooks(number)
	serviceHooks := env.Twilio.Webhooks(service)
	assert.Equal(t, twiliotest.Webhooks{}, numberHooks, "nothing is changed in the account")
	assert.Equal(t, twiliotest.Webhooks{}, serviceHooks, "nothing is changed in the account")
}

func TestSiteTwilioLeavesTheAddressToCopyWhenTheKeyMayNotChangeTheNumber(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	account := env.Twilio.AddAccount()
	key := env.Twilio.AddAPIKey(account)
	env.Twilio.Forbid(env.Twilio.AddNumber(account, testhelper.TwilioNumber))

	row := twilioCreated(t, createTwilio(t, c, testhelper.SiteTwilio(account.SID, key.SID, key.Secret, testhelper.TwilioNumber), account.AuthToken))

	assert.False(t, twilioConfigOf(t, row).CallbacksSet)
}

func TestSiteTwilioRefusesWhatTwilioRefuses(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	account := env.Twilio.AddAccount()
	key := env.Twilio.AddAPIKey(account)
	env.Twilio.AddNumber(account, testhelper.TwilioNumber)
	revoked := env.Twilio.AddAPIKey(account)
	env.Twilio.RevokeAPIKey(revoked)

	cases := []struct {
		name  string
		cfg   siteapi.IntegrationConfigInput
		token string
		field string
	}{
		{"a wrong secret", testhelper.SiteTwilio(account.SID, key.SID, "wrongsecret", testhelper.TwilioNumber), account.AuthToken, "config.secret"},
		{"a revoked key", testhelper.SiteTwilio(account.SID, revoked.SID, revoked.Secret, testhelper.TwilioNumber), account.AuthToken, "config.secret"},
		{"a number the account does not have", testhelper.SiteTwilio(account.SID, key.SID, key.Secret, "+14155552671"), account.AuthToken, "config.from"},
		{"a Messaging Service the account does not have", testhelper.SiteTwilio(account.SID, key.SID, key.Secret, "MG00000000000000000000000000000000"), account.AuthToken, "config.from"},
		{"an Account SID of the wrong shape", testhelper.SiteTwilio("AC123", key.SID, key.Secret, testhelper.TwilioNumber), account.AuthToken, "config.accountSid"},
		{"a sender that is none", testhelper.SiteTwilio(account.SID, key.SID, key.Secret, "not a sender!"), account.AuthToken, "config.from"},
		{"no Auth Token for the callbacks", testhelper.SiteTwilio(account.SID, key.SID, key.Secret, testhelper.TwilioNumber), "", "params.auth_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := problemOf(t, createTwilio(t, c, tc.cfg, tc.token))
			assert.Contains(t, errs, tc.field)
		})
	}

	t.Run("another Preset", func(t *testing.T) {
		res, err := c.SiteIntegrationsCreate(t.Context(), &siteapi.CreateIntegrationInput{
			Name: "Acme SMS", Config: testhelper.SiteTwilio(account.SID, key.SID, key.Secret, testhelper.TwilioNumber),
			Preset: siteapi.NewOptNilString("generic"),
		}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		assert.Contains(t, problemOf(t, res), "preset")
	})

	list, err := c.SiteIntegrationsList(t.Context(), siteapi.SiteIntegrationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	accounts := lo.Filter(*list.(*siteapi.SiteIntegrationsListOKApplicationJSON), func(row siteapi.SiteIntegrationResource, _ int) bool {
		return row.Provider == siteapi.SiteIntegrationProviderTwilio
	})
	assert.Len(t, accounts, 1, "a refused connect saved nothing: only the fixture account is there")
}

func TestSiteTwilioTheTwilioPresetIsNoOtherIntegrationsChoice(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteIntegrationsCreate(t.Context(), &siteapi.CreateIntegrationInput{
		Name: "relay", Config: smtpInput("smtp.example.com", "pw"),
		Preset: siteapi.NewOptNilString("twilio"), Params: siteapi.NewOptJsonObject(testhelper.TwilioAuthToken("token")),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Contains(t, problemOf(t, res), "preset")
}

func TestSiteTwilioRotatingTheWebhookSecretLeavesTheSendCredential(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	sms := env.ConnectTwilio(t)

	row := updated(t, updateTelegram(t, c, sms.Integration.ID, &siteapi.UpdateIntegrationInput{
		Params: siteapi.NewOptJsonObject(testhelper.TwilioAuthToken("rotatedauthtoken")),
	}))

	assert.Equal(t, []string{"auth_token"}, row.SecretsRetiring, "the old Auth Token stays accepted until retired")
	assert.Equal(t, twilioConfigOf(t, sms.Integration), twilioConfigOf(t, row), "the send credential is untouched")
	hooks := env.Twilio.Webhooks(sms.NumberSID)
	assert.Equal(t, row.HookUrl, hooks.Inbound, "the number still reaches the same hook")

	t.Run("the stored secret still reaches Twilio", func(t *testing.T) {
		service := env.Twilio.AddMessagingService(sms.Account)

		row := updated(t, updateTelegram(t, c, sms.Integration.ID, &siteapi.UpdateIntegrationInput{
			Config: siteapi.NewOptIntegrationConfigInput(testhelper.SiteTwilio(sms.Account.SID, sms.Key.SID, "", service)),
		}))

		assert.Equal(t, service, twilioConfigOf(t, row).From)
		hooks := env.Twilio.Webhooks(service)
		assert.Equal(t, row.HookUrl, hooks.Inbound)
	})

	t.Run("a blank secret is not kept for another key", func(t *testing.T) {
		other := env.Twilio.AddAPIKey(sms.Account)

		res := updateTelegram(t, c, sms.Integration.ID, &siteapi.UpdateIntegrationInput{
			Config: siteapi.NewOptIntegrationConfigInput(testhelper.SiteTwilio(sms.Account.SID, other.SID, "", testhelper.TwilioNumber)),
		})

		assert.Contains(t, problemOf(t, res), "config.secret")
	})

	t.Run("a revoked key is refused on update", func(t *testing.T) {
		env.Twilio.RevokeAPIKey(sms.Key)

		res := updateTelegram(t, c, sms.Integration.ID, &siteapi.UpdateIntegrationInput{
			Config: siteapi.NewOptIntegrationConfigInput(testhelper.SiteTwilio(sms.Account.SID, sms.Key.SID, sms.Key.Secret, testhelper.TwilioNumber)),
		})

		assert.Contains(t, problemOf(t, res), "config.secret")
	})
}

func TestSiteTwilioANewKeyRepointsTheNumber(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	sms := env.ConnectTwilio(t)

	res, err := c.SiteIntegrationsRegenerateKey(t.Context(), siteapi.SiteIntegrationsRegenerateKeyParams{WorkspaceId: fixtures.AcmePublicID, ID: sms.Integration.ID})
	require.NoError(t, err)
	row, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", res)

	assert.NotEqual(t, sms.Integration.HookUrl, row.HookUrl)
	hooks := env.Twilio.Webhooks(sms.NumberSID)
	assert.Equal(t, row.HookUrl, hooks.Inbound)
}

func updated(t *testing.T, res siteapi.SiteIntegrationsUpdateRes) *siteapi.SiteIntegrationResource {
	t.Helper()
	row, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", res)
	return row
}
