package testhelper

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
)

// TwilioNumber is the number the Twilio helpers send from, as the fake account holds it.
const TwilioNumber = "+15017250604"

// TwilioAccount is a Twilio account connected as an SMS Integration of Acme through the Site API,
// sending from TwilioNumber with an API key, as the fake REST API (env.Twilio) knows it.
type TwilioAccount struct {
	// Integration is the Integration as the create answered it.
	Integration *siteapi.SiteIntegrationResource
	Account     twiliotest.Account
	Key         twiliotest.APIKey
	// NumberSID is the SID of the number messages go out from.
	NumberSID string
}

// SiteTwilio is the Twilio config of an Integration write on the site API: an API key and its
// secret when keySID is set, else the account's Auth Token as secret.
func SiteTwilio(accountSID, keySID, secret, from string) siteapi.IntegrationConfigInput {
	c := siteapi.TwilioConfigInput{Kind: siteapi.TwilioConfigInputKindTwilio, AccountSid: accountSID, Secret: secret, From: from}
	if keySID != "" {
		c.ApiKeySid = siteapi.NewOptNilString(keySID)
	}
	return siteapi.IntegrationConfigInput{OneOf: siteapi.NewTwilioConfigInputIntegrationConfigInputSum(c)}
}

// TwilioAuthToken is the params of a Twilio Integration write: the Auth Token its callbacks are
// signed with.
func TwilioAuthToken(token string) siteapi.JsonObject {
	return siteapi.JsonObject{"auth_token": []byte(`"` + token + `"`)}
}

// The account of the fixture Integration IntegrationAcmeSms, as fixtures/integrations.yml stores
// it: it calls Twilio with its Auth Token, and sends from TwilioNumber.
const (
	AcmeTwilioAccountSID = "AC00000000000000000000000000000001"
	AcmeTwilioAuthToken  = "fixtureacmetwilioauthtoken000001"
)

// AcmeTwilio adds the account of the fixture Integration IntegrationAcmeSms and its TwilioNumber
// to the fake REST API and returns it: the SMS Integration of Acme a test sends through or posts
// callbacks to. It has no API key. A test of connecting an account connects one of its own
// (ConnectTwilio).
func (env *TestEnv) AcmeTwilio(t *testing.T) TwilioAccount {
	t.Helper()
	account := env.Twilio.AddAccountOf(AcmeTwilioAccountSID, AcmeTwilioAuthToken)
	number := env.Twilio.AddNumber(account, TwilioNumber)
	return TwilioAccount{Integration: env.acmeIntegration(t, fixtures.IntegrationAcmeSmsID), Account: account, NumberSID: number}
}

// The account of the fixture Integration IntegrationKrustySms, as fixtures/integrations.yml stores
// it, sending from TwilioNumber like Acme's.
const (
	KrustyTwilioAccountSID = "AC00000000000000000000000000007720"
	KrustyTwilioAuthToken  = "fixturekrustytwilioauthtoken7720"
)

// KrustyTwilio is AcmeTwilio for Krusty's fixture Integration IntegrationKrustySms: the SMS
// Integration of a Workspace whose audiences are no other test's.
func (env *TestEnv) KrustyTwilio(t *testing.T) TwilioAccount {
	t.Helper()
	account := env.Twilio.AddAccountOf(KrustyTwilioAccountSID, KrustyTwilioAuthToken)
	number := env.Twilio.AddNumber(account, TwilioNumber)
	return TwilioAccount{
		Integration: env.fixtureIntegration(t, fixtures.KrustyOwnerKentEmail, fixtures.KrustyPublicID, fixtures.IntegrationKrustySmsID),
		Account:     account, NumberSID: number,
	}
}

// The account of the fixture Integration IntegrationLacunaSms, as fixtures/integrations.yml stores
// it, sending from TwilioNumber like Acme's.
const (
	LacunaTwilioAccountSID = "AC00000000000000000000000000007740"
	LacunaTwilioAuthToken  = "fixturelacunatwilioauthtoken7740"
)

// LacunaTwilio is AcmeTwilio for Lacuna's fixture Integration IntegrationLacunaSms: the SMS
// Integration of the Workspace whose SMS Broadcasts go to every Contact it holds.
func (env *TestEnv) LacunaTwilio(t *testing.T) TwilioAccount {
	t.Helper()
	account := env.Twilio.AddAccountOf(LacunaTwilioAccountSID, LacunaTwilioAuthToken)
	number := env.Twilio.AddNumber(account, TwilioNumber)
	return TwilioAccount{
		Integration: env.fixtureIntegration(t, fixtures.LacunaOwnerLenaEmail, fixtures.LacunaPublicID, fixtures.IntegrationLacunaSmsID),
		Account:     account, NumberSID: number,
	}
}

// ConnectTwilio adds an account with an API key and TwilioNumber to the fake REST API and connects
// it as an SMS Integration of Acme, as its owner does in the app.
func (env *TestEnv) ConnectTwilio(t *testing.T) TwilioAccount {
	t.Helper()
	account := env.Twilio.AddAccount()
	key := env.Twilio.AddAPIKey(account)
	number := env.Twilio.AddNumber(account, TwilioNumber)
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsCreate(t.Context(), &siteapi.CreateIntegrationInput{
		Name:   "Acme SMS",
		Config: SiteTwilio(account.SID, key.SID, key.Secret, TwilioNumber),
		Params: siteapi.NewOptJsonObject(TwilioAuthToken(account.AuthToken)),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	row, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "connect the Twilio account: got %T", res)
	return TwilioAccount{Integration: row, Account: account, Key: key, NumberSID: number}
}

// PostTwilioCallback posts form to the hook URL as Twilio does: form-encoded, signed with
// authToken over the URL and the parameters. It returns the response.
func (env *TestEnv) PostTwilioCallback(t *testing.T, hookURL, authToken string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, strings.TrimPrefix(hookURL, env.AppURL), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", twiliotest.Sign(authToken, hookURL, form))
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// ReportTwilioStatus posts the status callback Twilio sends about a message it accepted from the
// account: to the StatusCallback the message was sent with, signed with the account's Auth Token,
// with Twilio's status and, when it gives one, its error code. It returns the response.
func (env *TestEnv) ReportTwilioStatus(t *testing.T, account TwilioAccount, msg twiliotest.Message, status, errorCode string) *httptest.ResponseRecorder {
	t.Helper()
	require.NotEmpty(t, msg.StatusCallback, "the message was sent without a StatusCallback")
	form := url.Values{
		"MessageSid": {msg.SID}, "SmsSid": {msg.SID}, "MessageStatus": {status}, "SmsStatus": {status},
		"AccountSid": {account.Account.SID}, "From": {TwilioNumber}, "To": {msg.To}, "ApiVersion": {"2010-04-01"},
	}
	if errorCode != "" {
		form.Set("ErrorCode", errorCode)
	}
	return env.PostTwilioCallback(t, msg.StatusCallback, account.Account.AuthToken, form)
}
