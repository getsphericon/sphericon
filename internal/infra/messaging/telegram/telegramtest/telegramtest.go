// Package telegramtest is a fake Telegram Bot API on httptest, for tests that reach Telegram the
// way production does: through the Instance's Bot API base URL. It knows the bots a test adds,
// records every method call with its parameters, and answers each call as Telegram would, or
// with a failure the test scripts.
//
// The harness (testhelper.Setup) starts one per test and points SPHERICON_TELEGRAM_BOT_API_URL
// at it, as env.Telegram.
package telegramtest

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Method names of the Bot API the fake answers.
const (
	GetMe               = "getMe"
	SetWebhook          = "setWebhook"
	DeleteWebhook       = "deleteWebhook"
	SendMessage         = "sendMessage"
	SendPhoto           = "sendPhoto"
	AnswerCallbackQuery = "answerCallbackQuery"
)

// Bot is a bot the fake knows.
type Bot struct {
	ID       int64
	Username string
}

// Request is one method call the fake received: the token it was made with, the method and its
// parameters as JSON decoded them (a number is a json.Number; a multipart field is a string).
type Request struct {
	Token  string
	Method string
	Params map[string]any
}

// String is the parameter name as a string, "" when it is absent.
func (c Request) String(name string) string {
	switch v := c.Params[name].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case nil:
		return ""
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

// Bool is the parameter name as a bool: false when it is absent, as Telegram reads it.
func (c Request) Bool(name string) bool {
	switch v := c.Params[name].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// Int is the parameter name as an int64, 0 when it is absent.
func (c Request) Int(name string) int64 {
	n, err := strconv.ParseInt(c.String(name), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// Strings is the parameter name as a list of strings (allowed_updates), nil when absent.
func (c Request) Strings(name string) []string {
	var out []string
	switch v := c.Params[name].(type) {
	case []any:
		for _, x := range v {
			s, _ := x.(string)
			out = append(out, s)
		}
	case string:
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return []string{v}
		}
	}
	return out
}

// Failure is an answer of Telegram that refuses a call.
type Failure struct {
	Code        int
	Description string
	RetryAfter  int
}

// The refusals the Bot API answers, in Telegram's own words. The recorded tests of package telegram
// check them against the real Bot API; Blocked, Deactivated and CantInitiate are not recorded.
var (
	// Unauthorized is a wrong or revoked token.
	Unauthorized = Failure{Code: http.StatusUnauthorized, Description: "Unauthorized"}
	// Blocked is a person who blocked the bot.
	Blocked = Failure{Code: http.StatusForbidden, Description: "Forbidden: bot was blocked by the user"}
	// Deactivated is a person who deleted their Telegram account.
	Deactivated = Failure{Code: http.StatusForbidden, Description: "Forbidden: user is deactivated"}
	// CantInitiate is a chat the bot may not start a conversation in: a 403 that is neither a block
	// nor a deactivated user.
	CantInitiate = Failure{Code: http.StatusForbidden, Description: "Forbidden: bot can't initiate conversation with a user"}
	// ChatNotFound is a chat id the bot cannot resolve.
	ChatNotFound = Failure{Code: http.StatusBadRequest, Description: "Bad Request: chat not found"}
	// BadEntities is a text whose formatting Telegram cannot parse.
	BadEntities = Failure{Code: http.StatusBadRequest, Description: `Bad Request: can't parse entities: Unsupported start tag "foo" at byte offset 0`}
	// BadWebhook is a webhook URL Telegram does not accept (not public HTTPS on an allowed port).
	BadWebhook = Failure{Code: http.StatusBadRequest, Description: "Bad Request: bad webhook: An HTTPS URL must be provided for webhook"}
	// NullReplyMarkup is a call whose reply_markup is JSON null: the parameter must be an object or
	// absent.
	NullReplyMarkup = Failure{Code: http.StatusBadRequest, Description: "Bad Request: object expected as reply markup"}
)

// TooManyRequests is a 429 that asks to wait seconds before the next call.
func TooManyRequests(seconds int) Failure {
	return Failure{Code: http.StatusTooManyRequests, Description: fmt.Sprintf("Too Many Requests: retry after %d", seconds), RetryAfter: seconds}
}

// BadRequest is a 400 with Telegram's description.
func BadRequest(description string) Failure {
	return Failure{Code: http.StatusBadRequest, Description: "Bad Request: " + description}
}

// script is a scripted failure: of a method, for one chat or any, once or until cleared.
type script struct {
	method string
	chatID string // "" for any chat
	once   bool
	fail   Failure
}

// Server is the fake Bot API.
type Server struct {
	srv   *httptest.Server
	start sync.Once

	mu        sync.Mutex
	bots      map[string]Bot // by token
	calls     []Request
	scripts   []script
	messageID int64
}

// New makes a fake Bot API, closed when the test ends. It listens at once but serves from the
// first call that scripts it (AddBot, Rotate, Revoke, UnknownToken, Fail…): a test that never uses
// it, such as one inside a testing/synctest bubble, runs no goroutine of its own.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{bots: map[string]Bot{}}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	tb.Cleanup(s.srv.Close)
	return s
}

// serving starts serving, once.
func (s *Server) serving() { s.start.Do(s.srv.Start) }

// URL is the base URL the fake serves the Bot API at, as SPHERICON_TELEGRAM_BOT_API_URL names it.
func (s *Server) URL() string { return "http://" + s.srv.Listener.Addr().String() }

// AddBot makes the fake know a bot with id and username (without the @) and returns a token of it,
// shaped as @BotFather issues one.
func (s *Server) AddBot(id int64, username string) string {
	s.serving()
	token := newToken(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bots[token] = Bot{ID: id, Username: username}
	return token
}

// AddBotToken makes the fake know a bot with id and username under a token the test names, such
// as the one a fixture Integration stores.
func (s *Server) AddBotToken(id int64, username, token string) {
	s.serving()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bots[token] = Bot{ID: id, Username: username}
}

// Rotate issues another token of the bot with id, as @BotFather's /revoke does: the old tokens
// stop working. It panics when the fake knows no such bot.
func (s *Server) Rotate(id int64) string {
	s.serving()
	s.mu.Lock()
	defer s.mu.Unlock()
	var bot Bot
	found := false
	for token, b := range s.bots {
		if b.ID == id {
			bot, found = b, true
			delete(s.bots, token)
		}
	}
	if !found {
		panic(fmt.Sprintf("telegramtest: no bot %d", id))
	}
	token := newToken(id)
	s.bots[token] = bot
	return token
}

// Revoke makes token stop working: every call with it answers 401.
func (s *Server) Revoke(token string) {
	s.serving()
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bots, token)
}

// UnknownToken is a token of the bot with id, shaped as @BotFather issues one, that the fake does
// not know: every call with it answers 401.
func (s *Server) UnknownToken(id int64) string {
	s.serving()
	return newToken(id)
}

// newToken is a random token of the bot with id, shaped as @BotFather issues one.
func newToken(id int64) string {
	return strconv.FormatInt(id, 10) + ":AA" + (rand.Text() + rand.Text())[:33]
}

// Fail makes every call of method answer f until Clear.
func (s *Server) Fail(method string, f Failure) { s.add(script{method: method, fail: f}) }

// FailOnce makes the next call of method answer f.
func (s *Server) FailOnce(method string, f Failure) {
	s.add(script{method: method, fail: f, once: true})
}

// FailChat makes every call of method to chatID (its chat_id parameter) answer f until Clear,
// while calls to other chats succeed.
func (s *Server) FailChat(method string, chatID int64, f Failure) {
	s.add(script{method: method, chatID: strconv.FormatInt(chatID, 10), fail: f})
}

// Clear drops every scripted failure.
func (s *Server) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts = nil
}

func (s *Server) add(sc script) {
	s.serving()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts = append(s.scripts, sc)
}

// Requests is every call the fake received, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// RequestsOf is every call of method, in order.
func (s *Server) RequestsOf(method string) []Request {
	var out []Request
	for _, c := range s.Requests() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// Last is the latest call of method; ok is false when there was none.
func (s *Server) Last(method string) (Request, bool) {
	calls := s.RequestsOf(method)
	if len(calls) == 0 {
		return Request{}, false
	}
	return calls[len(calls)-1], true
}

// maxBody bounds a call's body, an uploaded photo included.
const maxBody = 10 << 20

// pathOf is /bot<token>/<method>.
var pathOf = regexp.MustCompile(`^/bot([^/]+)/([A-Za-z]+)$`)

// secretToken is what Telegram accepts as a webhook's secret_token.
var secretToken = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	m := pathOf.FindStringSubmatch(r.URL.Path)
	if m == nil {
		answer(w, Failure{Code: http.StatusNotFound, Description: "Not Found"})
		return
	}
	params, err := readParams(r)
	if err != nil {
		answer(w, BadRequest("can't parse request: "+err.Error()))
		return
	}
	call := Request{Token: m[1], Method: m[2], Params: params}

	s.mu.Lock()
	s.calls = append(s.calls, call)
	bot, known := s.bots[call.Token]
	failure, failed := s.scripted(call)
	s.mu.Unlock()

	switch {
	case !known:
		answer(w, Unauthorized)
	case failed:
		answer(w, failure)
	default:
		s.succeed(w, call, bot)
	}
}

// scripted is the failure the call meets, consuming a one-time one. The caller holds mu.
func (s *Server) scripted(call Request) (Failure, bool) {
	for i, sc := range s.scripts {
		if sc.method != call.Method || (sc.chatID != "" && sc.chatID != call.String("chat_id")) {
			continue
		}
		if sc.once {
			s.scripts = slices.Delete(s.scripts, i, i+1)
		}
		return sc.fail, true
	}
	return Failure{}, false
}

func (s *Server) succeed(w http.ResponseWriter, call Request, bot Bot) {
	switch call.Method {
	case GetMe:
		ok(w, map[string]any{"id": bot.ID, "is_bot": true, "first_name": bot.Username, "username": bot.Username})
	case SetWebhook:
		if !secretToken.MatchString(call.String("secret_token")) && call.String("secret_token") != "" {
			answer(w, BadRequest("bad webhook: secret token contains unallowed characters"))
			return
		}
		if !strings.HasPrefix(call.String("url"), "https://") {
			answer(w, BadWebhook)
			return
		}
		ok(w, true)
	case DeleteWebhook, AnswerCallbackQuery:
		ok(w, true)
	case SendMessage, SendPhoto:
		if markup, present := call.Params["reply_markup"]; present && markup == nil {
			answer(w, NullReplyMarkup)
			return
		}
		s.mu.Lock()
		s.messageID++
		id := s.messageID
		s.mu.Unlock()
		chatID := call.Int("chat_id")
		ok(w, map[string]any{
			"message_id": id,
			"date":       1767225600,
			"chat":       map[string]any{"id": chatID, "type": "private"},
			"from":       map[string]any{"id": bot.ID, "is_bot": true, "first_name": bot.Username, "username": bot.Username},
		})
	default:
		answer(w, Failure{Code: http.StatusNotFound, Description: "Not Found: method not found"})
	}
}

// readParams decodes a call's parameters: a JSON object, a form or a multipart form, as the Bot
// API takes them. An empty body (or JSON null) has none.
func readParams(r *http.Request) (map[string]any, error) {
	params := map[string]any{}
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		mediaType = "application/json"
	}
	switch mediaType {
	case "multipart/form-data":
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, err
		}
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if part.FileName() != "" {
				params[part.FormName()] = "attach://" + part.FileName()
				continue
			}
			value, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			params[part.FormName()] = string(value)
		}
	case "application/x-www-form-urlencoded":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		for k, v := range form {
			params[k] = v[0]
		}
	default:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(body)) == 0 || string(bytes.TrimSpace(body)) == "null" {
			return params, nil
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		if err := dec.Decode(&params); err != nil {
			return nil, err
		}
	}
	return params, nil
}

func ok(w http.ResponseWriter, result any) {
	write(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

func answer(w http.ResponseWriter, f Failure) {
	body := map[string]any{"ok": false, "error_code": f.Code, "description": f.Description}
	if f.RetryAfter > 0 {
		body["parameters"] = map[string]any{"retry_after": f.RetryAfter}
	}
	write(w, f.Code, body)
}

func write(w http.ResponseWriter, code int, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(raw); err != nil {
		return
	}
}

// Webhook is the URL and secret the bot's last setWebhook call named, from the fake's record:
// what a test posts updates with. ok is false when no setWebhook of the bot was made.
func (s *Server) Webhook(botID int64) (hookURL, secret string, ok bool) {
	calls := s.Requests()
	for _, c := range slices.Backward(calls) {
		if c.Method != SetWebhook || !strings.HasPrefix(c.Token, strconv.FormatInt(botID, 10)+":") {
			continue
		}
		return c.String("url"), c.String("secret_token"), true
	}
	return "", "", false
}
