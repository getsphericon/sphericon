package telegram_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram"
)

// The recorded tests run the provider against answers of the real Bot API, kept in
// testdata/recorded/<test>.yaml: by default they replay them and reach no network. With
// SPHERICON_TELEGRAM_RECORD=1 (mise run record:telegram) they call Telegram with the recording bot
// (SPHERICON_TELEGRAM_RECORD_TOKEN) and its chat with a person (SPHERICON_TELEGRAM_RECORD_CHAT_ID),
// and write each file anew. Before a file is written, every value that identifies the bot or the
// person is replaced by the fake below, and the test fails if one is still in the file.
const (
	fakeBotID       = "123456789"
	fakeToken       = fakeBotID + ":AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	fakeChatID      = "42"
	fakeBotUsername = "sphericon_test_bot"
	fakeBotName     = "Sphericon Test"
	fakeFirstName   = "Ann"
	fakeLastName    = "Example"
	fakeUsername    = "ann_example"
)

// recording is a recorded test's bot, the chat with a person it sends to, and what getMe says
// of the bot: the fakes when replaying, the real ones while recording.
type recording struct {
	bot         *telegram.Bot
	chatID      string
	botID       int64
	botUsername string
	// transport is the recorder, for a second bot of the same test (another token).
	transport http.RoundTripper
}

// recorded is the bot of the test's recording: replayed, or reaching Telegram when recording.
func recorded(t *testing.T) recording {
	t.Helper()
	name := filepath.Join("testdata", "recorded", t.Name())
	token, chatID, secrets := fakeToken, fakeChatID, []secret(nil)
	me := bot{ID: 123456789, Username: fakeBotUsername}
	mode := recorder.ModeReplayOnly
	if os.Getenv("SPHERICON_TELEGRAM_RECORD") == "1" {
		mode = recorder.ModeRecordOnly
		token, chatID = os.Getenv("SPHERICON_TELEGRAM_RECORD_TOKEN"), os.Getenv("SPHERICON_TELEGRAM_RECORD_CHAT_ID")
		require.NotEmpty(t, token, "SPHERICON_TELEGRAM_RECORD_TOKEN is the recording bot's token")
		require.NotEmpty(t, chatID, "SPHERICON_TELEGRAM_RECORD_CHAT_ID is the recording bot's chat with a person")
		me, secrets = learnSecrets(t, token, chatID)
	}
	rec, err := recorder.New(name,
		recorder.WithMode(mode),
		recorder.WithSkipRequestLatency(true),
		recorder.WithHook(func(i *cassette.Interaction) error { scrub(i, secrets); return nil }, recorder.BeforeSaveHook),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, rec.Stop())
		if mode == recorder.ModeRecordOnly {
			requireNoSecrets(t, name+".yaml", secrets)
		}
	})
	b, err := telegram.New(telegram.DefaultAPIURL, token, telegram.WithTransport(rec))
	require.NoError(t, err)
	return recording{bot: b, chatID: chatID, botID: me.ID, botUsername: me.Username, transport: rec}
}

// secret is a value of the recording that identifies the bot or the person, and its fake.
type secret struct {
	pattern *regexp.Regexp
	fake    string
	// replacement is what a match becomes: the fake, with what the pattern matched around it.
	replacement string
}

// learnSecrets asks Telegram, outside the recording, what identifies the bot and the person: the
// bot's id and names from getMe, the person's names from getChat. The token goes first, as it
// holds the bot's id.
func learnSecrets(t *testing.T, token, chatID string) (bot, []secret) {
	t.Helper()
	var me bot
	call(t, token, "getMe", &me)
	var chat struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
	}
	call(t, token, "getChat?chat_id="+chatID, &chat)
	out := []secret{literal(token, fakeToken), number(strconv.FormatInt(me.ID, 10), fakeBotID), number(chatID, fakeChatID)}
	for _, v := range [][2]string{
		{me.Username, fakeBotUsername}, {me.FirstName, fakeBotName},
		{chat.Username, fakeUsername}, {chat.LastName, fakeLastName}, {chat.FirstName, fakeFirstName},
	} {
		if v[0] != "" {
			out = append(out, literal(v[0], v[1]))
		}
	}
	return me, out
}

// bot is what getMe says of a bot.
type bot struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

func literal(value, fake string) secret {
	return secret{pattern: regexp.MustCompile(regexp.QuoteMeta(value)), fake: fake, replacement: fake}
}

// number is an id, replaced only as a whole number, never inside a longer one.
func number(value, fake string) secret {
	return secret{pattern: regexp.MustCompile(`(^|[^0-9])` + regexp.QuoteMeta(value) + `([^0-9]|$)`), fake: fake, replacement: "${1}" + fake + "${2}"}
}

func (s secret) apply(v string) string { return s.pattern.ReplaceAllString(v, s.replacement) }

// call is one Bot API call of the recording's own, never recorded.
func call(t *testing.T, token, method string, result any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, telegram.DefaultAPIURL+"/bot"+token+"/"+method, http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "call %s", strings.SplitN(method, "?", 2)[0])
	defer func() { _ = resp.Body.Close() }()
	var answer struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&answer))
	require.True(t, answer.OK, "Telegram refused %s", strings.SplitN(method, "?", 2)[0])
	require.NoError(t, json.Unmarshal(answer.Result, result))
}

// scrub replaces the secrets in everything the interaction keeps, and sets the lengths again,
// which the replay matches.
func scrub(i *cassette.Interaction, secrets []secret) {
	clean := func(v string) string {
		for _, s := range secrets {
			v = s.apply(v)
		}
		return v
	}
	i.Request.URL = clean(i.Request.URL)
	i.Request.RequestURI = clean(i.Request.RequestURI)
	i.Request.Body = clean(i.Request.Body)
	i.Request.ContentLength = int64(len(i.Request.Body))
	for k, vs := range i.Request.Form {
		for n := range vs {
			i.Request.Form[k][n] = clean(vs[n])
		}
	}
	i.Response.Body = clean(i.Response.Body)
	i.Response.ContentLength = int64(len(i.Response.Body))
	for _, h := range []http.Header{i.Request.Headers, i.Response.Headers} {
		for k, vs := range h {
			for n := range vs {
				h[k][n] = clean(vs[n])
			}
		}
	}
	if i.Response.Headers.Get("Content-Length") != "" {
		i.Response.Headers.Set("Content-Length", strconv.Itoa(len(i.Response.Body)))
	}
}

// requireNoSecrets fails when a real value is still in the written file.
func requireNoSecrets(t *testing.T, file string, secrets []secret) {
	t.Helper()
	written, err := os.ReadFile(file)
	require.NoError(t, err)
	for _, s := range secrets {
		if s.pattern.Match(written) {
			t.Errorf("%s still holds the real value of %q: delete the file", file, s.fake)
		}
	}
}
