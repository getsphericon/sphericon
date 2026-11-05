package testhelper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// TelegramBot is a fixture Telegram Integration and its bot, as the fake Bot API
// (env.Telegram) knows it.
type TelegramBot struct {
	// Integration is the Integration as the Site API reads it.
	Integration *siteapi.SiteIntegrationResource
	ID          int64
	Username    string
	Token       string
	// HookPath is the path of the Integration's Ingest hook, and Secret the webhook secret
	// Telegram sends back with every update: what PostTelegramUpdate posts with.
	HookPath string
	Secret   string
}

// The bot of the fixture Integration IntegrationAcmeTelegram, as fixtures/integrations.yml stores it.
const (
	AcmeTelegramBotID    = 7001
	AcmeTelegramUsername = "acme_fixture_bot"
	AcmeTelegramToken    = "7001:AAfixturetokenacmebot00000000000000"
	AcmeTelegramSecret   = "fixture-acme-webhook-secret"
)

// AcmeTelegram adds the bot of the fixture Integration IntegrationAcmeTelegram to the fake Bot API
// and returns it: the Telegram Integration of Acme a test sends through, binds on or posts updates
// to. It is not the default of its Channel (MakeDefaultTelegram). A test of connecting a bot
// connects one of its own through the API.
func (env *TestEnv) AcmeTelegram(t *testing.T) TelegramBot {
	t.Helper()
	return env.fixtureTelegram(t, fixtures.IntegrationAcmeTelegramID, TelegramBot{
		ID: AcmeTelegramBotID, Username: AcmeTelegramUsername, Token: AcmeTelegramToken,
		HookPath: "/hooks/" + fixtures.IntegrationAcmeTelegramIngestKey, Secret: AcmeTelegramSecret,
	})
}

// The bot of the fixture Integration IntegrationAcmeSecondTelegram, as fixtures/integrations.yml
// stores it.
const (
	AcmeSecondTelegramBotID    = 7800
	AcmeSecondTelegramUsername = "acme_second_fixture_bot"
	AcmeSecondTelegramToken    = "7800:AAfixturetokenacmesecondbot00000000"
	AcmeSecondTelegramSecret   = "fixture-acme-second-webhook-secret"
)

// AcmeSecondTelegram adds the bot of the fixture Integration IntegrationAcmeSecondTelegram to the
// fake Bot API and returns it: a Telegram Integration of Acme besides AcmeTelegram's, for a test
// that needs two bots. It is not the default of its Channel and holds no Binding.
func (env *TestEnv) AcmeSecondTelegram(t *testing.T) TelegramBot {
	t.Helper()
	return env.fixtureTelegram(t, fixtures.IntegrationAcmeSecondTelegramID, TelegramBot{
		ID: AcmeSecondTelegramBotID, Username: AcmeSecondTelegramUsername, Token: AcmeSecondTelegramToken,
		HookPath: "/hooks/" + fixtures.IntegrationAcmeSecondTelegramIngestKey, Secret: AcmeSecondTelegramSecret,
	})
}

// The bot of the fixture Integration IntegrationLacunaTelegram, as fixtures/integrations.yml stores it.
const (
	LacunaTelegramBotID    = 7741
	LacunaTelegramUsername = "lacuna_fixture_bot"
	LacunaTelegramToken    = "7741:AAfixturetokenlacunabot000000000000"
	LacunaTelegramSecret   = "fixture-lacuna-webhook-secret"
)

// LacunaTelegram is AcmeTelegram for Lacuna's fixture Integration IntegrationLacunaTelegram: the bot
// of the Workspace whose Telegram Broadcasts go to every Contact it holds, with the fixture's
// Bindings on it.
func (env *TestEnv) LacunaTelegram(t *testing.T) TelegramBot {
	t.Helper()
	env.Telegram.AddBotToken(LacunaTelegramBotID, LacunaTelegramUsername, LacunaTelegramToken)
	return TelegramBot{
		Integration: env.fixtureIntegration(t, fixtures.LacunaOwnerLenaEmail, fixtures.LacunaPublicID, fixtures.IntegrationLacunaTelegramID),
		ID:          LacunaTelegramBotID, Username: LacunaTelegramUsername, Token: LacunaTelegramToken,
		HookPath: "/hooks/" + fixtures.IntegrationLacunaTelegramIngestKey, Secret: LacunaTelegramSecret,
	}
}

// fixtureTelegram adds the bot of a fixture Telegram Integration of Acme to the fake Bot API and
// returns it with the Integration as the Site API reads it.
func (env *TestEnv) fixtureTelegram(t *testing.T, integrationID int64, bot TelegramBot) TelegramBot {
	t.Helper()
	env.Telegram.AddBotToken(bot.ID, bot.Username, bot.Token)
	bot.Integration = env.acmeIntegration(t, integrationID)
	return bot
}

// acmeIntegration is a fixture Integration of Acme as the Site API reads it to its owner.
func (env *TestEnv) acmeIntegration(t *testing.T, id int64) *siteapi.SiteIntegrationResource {
	t.Helper()
	return env.fixtureIntegration(t, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, id)
}

// fixtureIntegration is a fixture Integration of the Workspace as the Site API reads it to owner.
func (env *TestEnv) fixtureIntegration(t *testing.T, owner, workspace string, id int64) *siteapi.SiteIntegrationResource {
	t.Helper()
	row, err := env.SiteActor(t, owner).SiteIntegrationsGet(t.Context(),
		siteapi.SiteIntegrationsGetParams{WorkspaceId: siteapi.WorkspaceId(workspace), ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	return row
}

// PostTelegramUpdate posts a Bot API update (its JSON) to the bot's Ingest hook as Telegram does,
// with the webhook's secret header, and returns the response.
func (env *TestEnv) PostTelegramUpdate(t *testing.T, bot TelegramBot, update string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, bot.HookPath, strings.NewReader(update))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", bot.Secret)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

// BindTelegram binds the Telegram user to the Contact on the bot as a person does: a start link
// is issued for the Contact and the user sends the bot its `/start <token>`. The chat is the
// user's private one, and the update ids are the ones the call takes from updateID.
func (env *TestEnv) BindTelegram(t *testing.T, bot TelegramBot, contactID, userID, updateID int64) {
	t.Helper()
	botID := externalapi.EntityId(bot.Integration.ID)
	link, err := env.ExternalScoped(t, "contacts:write").TelegramStartLinksIssue(t.Context(),
		&externalapi.IssueTelegramStartLinkInput{IntegrationId: externalapi.NewOptNilEntityId(botID)},
		externalapi.TelegramStartLinksIssueParams{ContactId: externalapi.EntityId(contactID)})
	require.NoError(t, err)
	u, err := url.Parse(link.URL)
	require.NoError(t, err)
	env.PostTelegramUpdate(t, bot, fmt.Sprintf(
		`{"update_id":%d,"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, userID, userID, "/start "+u.Query().Get("start")))
}

// MakeDefaultTelegram makes the bot the default Telegram Integration of Acme: the one a merge tag
// or the tracker issues start links for.
func (env *TestEnv) MakeDefaultTelegram(t *testing.T, bot TelegramBot) {
	t.Helper()
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Integration().UpdateOneID(int64(bot.Integration.ID)).SetIsDefault(true).Exec(t.Context()))
}

// StartTelegram posts the update of person `from` pressing Start with the link's token to the bot.
func (env *TestEnv) StartTelegram(t *testing.T, bot TelegramBot, updateID, from int64, link string) {
	t.Helper()
	u, err := url.Parse(link)
	require.NoError(t, err)
	update := fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, from, from, "/start "+u.Query().Get("start"))
	require.Equal(t, http.StatusOK, env.PostTelegramUpdate(t, bot, update).Code)
}
