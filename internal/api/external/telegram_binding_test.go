package external_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// startUpdate is the update Telegram posts when person `from` presses Start on a link: a private
// chat whose id is the user's.
func startUpdate(updateID, from int64, text string) string {
	return fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, from, from, text)
}

// issue issues a start link for the Contact on the bot (the default bot when it is nil) and
// returns the token in it and the resource.
func issue(t *testing.T, c *externalapi.Client, contactID int64, integrationID *int64) (string, *externalapi.TelegramStartLinkResource) {
	t.Helper()
	in := &externalapi.IssueTelegramStartLinkInput{}
	if integrationID != nil {
		in.IntegrationId = externalapi.NewOptNilEntityId(externalapi.EntityId(*integrationID))
	}
	res, err := c.TelegramStartLinksIssue(t.Context(), in, externalapi.TelegramStartLinksIssueParams{ContactId: entityIDString(contactID)})
	require.NoError(t, err)
	link, ok := res.(*externalapi.TelegramStartLinkResource)
	require.Truef(t, ok, "got %T", res)
	u, err := url.Parse(link.URL)
	require.NoError(t, err)
	return u.Query().Get("start"), link
}

// onBot names the bot a link is issued for.
func onBot(bot testhelper.TelegramBot) *int64 {
	id := int64(bot.Integration.ID)
	return &id
}

func bindingsOf(t *testing.T, env *testhelper.TestEnv, destination string) []string {
	t.Helper()
	rows := env.DB.Binding.Query().Where(binding.Destination(destination)).AllX(t.Context())
	out := make([]string, 0, len(rows))
	for _, b := range rows {
		out = append(out, fmt.Sprintf("contact:%d", *b.ContactID))
	}
	return out
}

func TestAStartLinkBindsTheChatToItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, "contacts:write")

	token, link := issue(t, c, fixtures.ContactBobID, onBot(bot))
	posted := env.PostTelegramUpdate(t, bot, startUpdate(900, 555, "/start "+token))
	redelivered := env.PostTelegramUpdate(t, bot, startUpdate(900, 555, "/start "+token))

	assert.Equal(t, "https://t.me/"+testhelper.AcmeTelegramUsername+"?start="+token, link.URL)
	assert.Regexp(t, `^[A-Za-z0-9_-]{1,64}$`, token)
	assert.Equal(t, http.StatusOK, posted.Code)
	assert.Equal(t, http.StatusOK, redelivered.Code)
	assert.Equal(t, []string{fmt.Sprintf("contact:%d", fixtures.ContactBobID)}, bindingsOf(t, env, "555"))
	bound := env.QueuedCount(t, events.NameTelegramBound, map[string]any{"contactId": fixtures.ContactBobID, "integrationId": bot.Integration.ID})
	assert.Equal(t, 1, bound, "a redelivered update records nothing more")
}

func TestAStartLinkUsesTheDefaultBotWhenNoneIsNamed(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, "contacts:write", "integrations:write")
	made, err := c.IntegrationsUpdate(t.Context(), &externalapi.UpdateIntegrationInput{IsDefault: externalapi.NewOptBool(true)},
		externalapi.IntegrationsUpdateParams{ID: externalapi.EntityId(bot.Integration.ID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.IntegrationResource{}, made)

	_, link := issue(t, c, fixtures.ContactBobID, nil)

	assert.Contains(t, link.URL, "https://t.me/"+testhelper.AcmeTelegramUsername+"?start=")
}

func TestAStartLinkLapsesAfterThirtyDays(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, "contacts:write")
	token, link := issue(t, c, fixtures.ContactBobID, onBot(bot))
	assert.WithinDuration(t, now.Add(30*24*time.Hour), time.Time(link.ExpiresAt), time.Second)

	now = now.Add(30*24*time.Hour + time.Minute)
	env.PostTelegramUpdate(t, bot, startUpdate(901, 556, "/start "+token))

	assert.Empty(t, bindingsOf(t, env, "556"))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramBound, nil))
}

func TestAStartLinkWorksOnce(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, "contacts:write")
	token, _ := issue(t, c, fixtures.ContactBobID, onBot(bot))

	env.PostTelegramUpdate(t, bot, startUpdate(902, 557, "/start "+token))
	env.PostTelegramUpdate(t, bot, startUpdate(903, 558, "/start "+token))

	assert.Empty(t, bindingsOf(t, env, "558"), "the second chat to use the token binds nothing")
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramBound, nil))
}

func TestAnotherBotsStartLinkBindsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	first := env.AcmeTelegram(t)
	second := env.AcmeSecondTelegram(t)
	c := env.ExternalScoped(t, "contacts:write")
	firstID := int64(first.Integration.ID)
	token, _ := issue(t, c, fixtures.ContactBobID, &firstID)

	env.PostTelegramUpdate(t, second, startUpdate(904, 559, "/start "+token))

	assert.Empty(t, bindingsOf(t, env, "559"))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramBound, nil))
}

func TestABareStartBindsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)

	env.PostTelegramUpdate(t, bot, startUpdate(905, 560, "/start"))
	env.PostTelegramUpdate(t, bot, startUpdate(906, 560, "/start not-a-token"))

	assert.Empty(t, bindingsOf(t, env, "560"))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramBound, nil))
}

func TestAStartInAGroupBindsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, "contacts:write")
	token, _ := issue(t, c, fixtures.ContactBobID, onBot(bot))
	inGroup := fmt.Sprintf(`{"update_id":907,"message":{"message_id":1,"date":1767225600,"chat":{"id":-1001,"type":"supergroup"},"from":{"id":561,"is_bot":false},"text":"/start %s"}}`, token)

	posted := env.PostTelegramUpdate(t, bot, inGroup)

	assert.Equal(t, http.StatusOK, posted.Code)
	assert.Empty(t, bindingsOf(t, env, "561"))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramBound, nil))
}

func TestAChatThatStartsWithAnotherContactsTokenMovesToThatContact(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	c := env.ExternalScoped(t, "contacts:write")
	bobs, _ := issue(t, c, fixtures.ContactBobID, onBot(bot))
	carols, _ := issue(t, c, fixtures.ContactCarolID, onBot(bot))

	env.PostTelegramUpdate(t, bot, startUpdate(908, 562, "/start "+bobs))
	env.PostTelegramUpdate(t, bot, startUpdate(909, 562, "/start "+carols))

	assert.Equal(t, []string{fmt.Sprintf("contact:%d", fixtures.ContactCarolID)}, bindingsOf(t, env, "562"))
	byBob := map[string]any{"contactId": fixtures.ContactBobID}
	byCarol := map[string]any{"contactId": fixtures.ContactCarolID}
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnbound, byBob), "the old Contact is told")
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramBound, byCarol))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramUnbound, byCarol))
}

func TestStartLinkRefusals(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	issueOn := func(contactID int64, integrationID *int64) any {
		in := &externalapi.IssueTelegramStartLinkInput{}
		if integrationID != nil {
			in.IntegrationId = externalapi.NewOptNilEntityId(externalapi.EntityId(*integrationID))
		}
		res, err := c.TelegramStartLinksIssue(t.Context(), in, externalapi.TelegramStartLinksIssueParams{ContactId: entityIDString(contactID)})
		require.NoError(t, err)
		return res
	}
	email := int64(fixtures.IntegrationAcmeDefaultID)

	noBot := issueOn(fixtures.ContactBobID, nil)
	notABot := issueOn(fixtures.ContactBobID, &email)
	noContact := issueOn(999999, nil)

	assert.IsType(t, &externalapi.TelegramStartLinksIssueUnprocessableEntity{}, noBot, "no default Telegram integration")
	assert.IsType(t, &externalapi.TelegramStartLinksIssueUnprocessableEntity{}, notABot, "an email integration is no bot")
	assert.IsType(t, &externalapi.TelegramStartLinksIssueNotFound{}, noContact)
}
