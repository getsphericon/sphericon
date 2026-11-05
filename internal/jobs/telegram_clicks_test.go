package jobs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/telegramrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Clicks in Telegram messages (ADR 0083): every link of the text and every button URL of a sent
// message is a short link on the Tracking domain or the Instance's host, and following one records
// telegram.clicked before it redirects.

const (
	textLink   = "https://shop.test/offer?a=1&b=2"
	buttonLink = "https://shop.test/button"
	telegramUA = "TelegramBot (like TwitterBot)"
	browserUA  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
)

var (
	hrefOf  = regexp.MustCompile(`<a href="([^"]+)">`)
	shortOf = regexp.MustCompile(`^https?://[^/]+(/c/[A-Za-z0-9_-]+)$`)
)

// shortPaths are the `/c/<token>` paths of the links of the only message the bot got: the hrefs of
// its text first, then its buttons' URLs.
func shortPaths(t *testing.T, env *testhelper.TestEnv, chatID string) (text, buttons []string) {
	t.Helper()
	sends := lo.Filter(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) bool { return r.String("chat_id") == chatID })
	require.Len(t, sends, 1)
	path := func(url string) string {
		m := shortOf.FindStringSubmatch(strings.ReplaceAll(url, "&amp;", "&"))
		require.NotNil(t, m, "%q is a short link", url)
		return m[1]
	}
	for _, m := range hrefOf.FindAllStringSubmatch(sends[0].String("text"), -1) {
		text = append(text, path(m[1]))
	}
	if markup := sends[0].String("reply_markup"); markup != "" {
		var kb struct {
			Rows [][]struct {
				URL string `json:"url"`
			} `json:"inline_keyboard"`
		}
		require.NoError(t, json.Unmarshal([]byte(markup), &kb))
		for _, row := range kb.Rows {
			for _, b := range row {
				buttons = append(buttons, path(b.URL))
			}
		}
	}
	return text, buttons
}

func follow(t *testing.T, env *testhelper.TestEnv, path, userAgent string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Forwarded-For", "192.0.2.10")
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func TestALinkAndAButtonOfASentTelegramMessageAreShortLinksThatRedirectAndRecordAClick(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	step := telegramStep(bot, `Read <a href="`+textLink+`">the offer</a> now`)
	step.Buttons = []telegramrender.Button{{Label: "Open", URL: buttonLink}}
	automationID := telegramAutomation(t, env, step)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 701001, 701001)
	enrollVia(t, env, automationID, fixtures.ContactBobID)
	runSteps(t, env)

	text, buttons := shortPaths(t, env, "701001")
	require.Len(t, text, 1)
	require.Len(t, buttons, 1)
	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	assert.NotContains(t, sends[0].String("text"), "shop.test", "the destination does not appear in the message")
	assert.NotContains(t, sends[0].String("reply_markup"), "shop.test")

	for path, target := range map[string]string{text[0]: textLink, buttons[0]: buttonLink} {
		resp := follow(t, env, path, browserUA)
		assert.Equal(t, http.StatusFound, resp.Code)
		assert.Equal(t, target, resp.Header().Get("Location"))
		assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramClicked, map[string]any{
			"contactId": fixtures.ContactBobID, "url": target, "automationId": automationID, "integrationId": bot.Integration.ID,
		}), target)
	}
	clicks := env.QueuedEvents(t, events.NameTelegramClicked)
	require.Len(t, clicks, 2)
	assert.NotZero(t, clicks[0].(*events.TelegramClicked).OutboundMessageID)
	assert.False(t, clicks[0].(*events.TelegramClicked).Machine)
}

func TestTheSameUrlOfAMessageKeepsOneShortLink(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	step := telegramStep(bot, `<a href="`+buttonLink+`">one</a> and <a href="`+buttonLink+`">two</a>`)
	step.Buttons = []telegramrender.Button{{Label: "Open", URL: buttonLink}}
	automationID := telegramAutomation(t, env, step)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 702001, 702001)
	enrollVia(t, env, automationID, fixtures.ContactBobID)
	runSteps(t, env)

	text, buttons := shortPaths(t, env, "702001")

	assert.Equal(t, []string{text[0], text[0]}, text)
	assert.Equal(t, text[:1], buttons, "the text and the button share the token")
	assert.Equal(t, 1, env.DB.TrackedLink.Query().Where(trackedlink.URL(buttonLink)).CountX(t.Context()))
}

func TestTheUnsubscribeLinkOfATelegramMessageIsNotTracked(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	automationID := telegramAutomation(t, env, telegramStep(bot, `<a href="`+textLink+`">offer</a> <a href="{{ unsubscribe_url }}">stop</a>`))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 703001, 703001)
	enrollVia(t, env, automationID, fixtures.ContactBobID)
	runSteps(t, env)

	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	hrefs := hrefOf.FindAllStringSubmatch(sends[0].String("text"), -1)
	require.Len(t, hrefs, 2)
	assert.Contains(t, hrefs[0][1], "/c/")
	assert.Contains(t, hrefs[1][1], "/e/u/", "the opt-out link goes straight to the unsubscribe page")
}

func TestAnUnknownShortLinkIsNotFoundAndRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	resp := follow(t, env, "/c/nosuchtoken", browserUA)

	assert.Equal(t, http.StatusNotFound, resp.Code)
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramClicked, nil))
}

func TestTelegramsLinkPreviewFetcherIsMachineEngagementAndAPersonSetsTheRollupAndTheFigure(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	id := telegramBroadcastOf(t, env, bot, broadcasts.TelegramContentInput{
		Text:    new(`<a href="` + textLink + `">offer</a>`),
		Buttons: &[]telegramrender.Button{{Label: "Open", URL: buttonLink}},
	})
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))
	text, buttons := shortPaths(t, env, "70001") // Liam's fixture chat
	clicked := func() bool {
		return env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.ContactID(fixtures.ContactLiamID)).OnlyX(ctx).ClickedAt != nil
	}

	resp := follow(t, env, text[0], telegramUA)
	assert.Equal(t, http.StatusFound, resp.Code, "the preview fetcher is redirected like anyone")
	assert.Equal(t, textLink, resp.Header().Get("Location"))
	assert.False(t, clicked(), "a machine click sets no rollup")
	assert.Zero(t, figuresOf(t, env, id).Clicked)

	follow(t, env, buttons[0], browserUA)
	assert.True(t, clicked(), "a person's click sets the recipient's rollup")
	assert.Equal(t, 1, figuresOf(t, env, id).Clicked)
	report, err := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()).Report(ctx, env.DB.Scoped(fixtures.AcmeID), id)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Clicked, "the Telegram report shows clicked")
	assert.Nil(t, report.Opened)

	clicks := env.QueuedEvents(t, events.NameTelegramClicked)
	require.Len(t, clicks, 2)
	assert.True(t, clicks[0].(*events.TelegramClicked).Machine)
	assert.False(t, clicks[1].(*events.TelegramClicked).Machine)
	assert.Equal(t, id, clicks[1].(*events.TelegramClicked).BroadcastID)
}

// telegram.clicked is a Trigger like email.clicked: a person's click enrolls the Contact, and a hit
// by software starts nothing (ADR 0053, 0083). Fixture: the Jobs greeting Automation, its live
// version's Trigger turned to telegram.clicked.
func TestAnAutomationTriggeredByATelegramClickIgnoresAMachineClick(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	greeting := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationJobsGreetingID)).OnlyX(ctx)
	env.DB.AutomationVersion.UpdateOneID(*greeting.LiveVersionID).SetTriggerEvent(events.NameTelegramClicked).ExecX(ctx)
	bot := env.AcmeTelegram(t)
	sender := telegramAutomation(t, env, telegramStep(bot, `<a href="`+textLink+`">offer</a>`))
	enrollVia(t, env, sender, fixtures.ContactLiamID)
	runSteps(t, env)
	text, _ := shortPaths(t, env, "70001") // Liam's fixture chat
	consume := func() []jobs.EvaluateTriggerArgs {
		envelopes := env.QueuedEnvelopes(t, events.NameTelegramClicked)
		env.WorkAutomationJobOf(t, envelopes[len(envelopes)-1])
		return queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind())
	}

	follow(t, env, text[0], telegramUA)
	assert.Empty(t, consume(), "the preview fetcher starts nothing")

	follow(t, env, text[0], browserUA)
	triggers := consume()
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.Bus).Work(ctx, job(triggers[0])))
	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactLiamID)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameTelegramClicked, *run.EventAction)
	assert.Equal(t, textLink, run.EventProperties["url"])
}
