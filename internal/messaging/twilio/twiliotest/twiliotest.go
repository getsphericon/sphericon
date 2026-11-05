// Package twiliotest is a fake Twilio REST API on httptest, for tests that reach Twilio the way
// production does: through the Instance's Twilio API base URL. It knows the accounts, API keys,
// numbers and Messaging Services a test adds, checks the credentials of every call as Twilio does,
// records every call, keeps the webhooks a call sets and the messages it accepts. It also signs a callback as Twilio signs
// the ones it posts.
//
// The harness (testhelper.Setup) starts one per test and points SPHERICON_TWILIO_API_URL at it, as
// env.Twilio. Every SID and secret it hands out is random, made when the test runs, except an
// account a test adds by its own SID and Auth Token (AddAccountOf).
package twiliotest

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// Account is an account the fake knows: its SID and its Auth Token.
type Account struct {
	SID       string
	AuthToken string
}

// APIKey is an API key of an account: its SID and its secret.
type APIKey struct {
	SID    string
	Secret string
}

// Webhooks is where Twilio posts for a number or a Messaging Service, as the calls set it.
type Webhooks struct {
	// Inbound is a number's SmsUrl or a Messaging Service's InboundRequestUrl.
	Inbound string
	// Status is a Messaging Service's StatusCallback; a number has none for messages.
	Status string
	// UseInboundWebhookOnNumber is a Messaging Service's setting of that name.
	UseInboundWebhookOnNumber string
}

// Message is a message the fake accepted: the account it was sent on, the SID it gave it, and the
// parameters of the call that created it.
type Message struct {
	Account             string
	SID                 string
	To                  string
	From                string
	MessagingServiceSID string
	Body                string
	StatusCallback      string
}

type sender struct {
	account  string
	sid      string
	number   string
	webhooks Webhooks
}

// Server is the fake API.
type Server struct {
	srv   *httptest.Server
	start sync.Once

	mu        sync.Mutex
	accounts  map[string]Account
	keys      map[string]struct{ account, secret string }
	numbers   map[string]*sender // by SID
	services  map[string]*sender // by SID
	forbidden map[string]bool    // sender SIDs the credentials may read but not change
	messages  []Message
	// refusal is what the next message create answers instead of accepting it, nil to accept;
	// refuseOnce answers it once.
	refusal    *Refusal
	refuseOnce bool
}

// Refusal is how Twilio refuses a call: the HTTP status, its error code and its message.
type Refusal struct {
	Status  int
	Code    int
	Message string
}

// The refusals of a message create a test scripts, in Twilio's own words
// (https://www.twilio.com/docs/api/errors/<code>).
var (
	// TooManyRequests is the account over its REST concurrency.
	TooManyRequests = Refusal{Status: http.StatusTooManyRequests, Code: 20429, Message: "Too Many Requests"}
	// QueueFull is the From's queue at capacity.
	QueueFull = Refusal{Status: http.StatusBadRequest, Code: 21611, Message: "This 'From' number has exceeded the maximum number of queued messages"}
	// AccountNotActive is a suspended account.
	AccountNotActive = Refusal{Status: http.StatusBadRequest, Code: 20005, Message: "Account not active"}
	// InvalidTo is a To that is no valid phone number.
	InvalidTo = Refusal{Status: http.StatusBadRequest, Code: 21211, Message: "Invalid 'To' Phone Number"}
	// Unsubscribed is a To that texted STOP to the sender: Twilio refuses it until it texts START.
	Unsubscribed = Refusal{Status: http.StatusBadRequest, Code: 21610, Message: "Attempt to send to unsubscribed recipient"}
	// NotMobile is a To that is no mobile number (a landline).
	NotMobile = Refusal{Status: http.StatusBadRequest, Code: 21614, Message: "'To' number is not a valid mobile number"}
	// Unknown is a code no table names.
	Unknown = Refusal{Status: http.StatusBadRequest, Code: 29999, Message: "Something Twilio does not explain"}
	// FromNotCapable is a From that cannot send to the To: a number that is not message-capable,
	// or an alphanumeric sender ID the To's country does not take.
	FromNotCapable = Refusal{Status: http.StatusBadRequest, Code: 21606, Message: "The 'From' phone number provided is not a valid message-capable Twilio phone number for this destination"}
)

// RefuseSends makes every message create answer r, after the credentials are checked, until
// AcceptSends.
func (s *Server) RefuseSends(r Refusal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refusal, s.refuseOnce = &r, false
}

// RefuseNextSend makes the next message create answer r; the ones after it are accepted.
func (s *Server) RefuseNextSend(r Refusal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refusal, s.refuseOnce = &r, true
}

// AcceptSends drops a scripted refusal.
func (s *Server) AcceptSends() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refusal = nil
}

// New makes a fake API, closed when the test ends. It listens at once but serves from the first
// account a test adds: a test that never uses it, such as one inside a testing/synctest bubble,
// runs no goroutine of its own.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{
		accounts:  map[string]Account{},
		keys:      map[string]struct{ account, secret string }{},
		numbers:   map[string]*sender{},
		services:  map[string]*sender{},
		forbidden: map[string]bool{},
	}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	tb.Cleanup(s.srv.Close)
	return s
}

// serving starts serving, once.
func (s *Server) serving() { s.start.Do(s.srv.Start) }

// URL is the base URL of the fake: SPHERICON_TWILIO_API_URL.
func (s *Server) URL() string { return "http://" + s.srv.Listener.Addr().String() }

func sid(prefix string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// AddAccount adds an account with a fresh SID and Auth Token.
func (s *Server) AddAccount() Account {
	s.serving()
	s.mu.Lock()
	defer s.mu.Unlock()
	a := Account{SID: sid("AC"), AuthToken: strings.TrimPrefix(sid("x"), "x")}
	s.accounts[a.SID] = a
	return a
}

// AddAccountOf adds an account with the SID and Auth Token the test names, such as the ones a
// fixture Integration stores.
func (s *Server) AddAccountOf(accountSID, authToken string) Account {
	s.serving()
	s.mu.Lock()
	defer s.mu.Unlock()
	a := Account{SID: accountSID, AuthToken: authToken}
	s.accounts[a.SID] = a
	return a
}

// AddAPIKey adds an API key of the account.
func (s *Server) AddAPIKey(account Account) APIKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := APIKey{SID: sid("SK"), Secret: strings.TrimPrefix(sid("x"), "x")}
	s.keys[k.SID] = struct{ account, secret string }{account.SID, k.Secret}
	return k
}

// RevokeAPIKey deletes the key: Twilio refuses it from then on.
func (s *Server) RevokeAPIKey(key APIKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, key.SID)
}

// AddNumber adds a phone number (E.164) to the account and returns its SID.
func (s *Server) AddNumber(account Account, number string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := &sender{account: account.SID, sid: sid("ON"), number: number}
	s.numbers[n.sid] = n
	return n.sid
}

// AddMessagingService adds a Messaging Service to the account and returns its SID.
func (s *Server) AddMessagingService(account Account) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &sender{account: account.SID, sid: sid("MG")}
	s.services[m.sid] = m
	return m.sid
}

// Forbid makes a change of the number or Messaging Service of that SID answer 403, as Twilio
// answers credentials that may read it but not change it.
func (s *Server) Forbid(senderSID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forbidden[senderSID] = true
}

// Webhooks is where Twilio posts for the number or Messaging Service of that SID.
func (s *Server) Webhooks(senderSID string) Webhooks {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.numbers[senderSID]; ok {
		return n.webhooks
	}
	if m, ok := s.services[senderSID]; ok {
		return m.webhooks
	}
	return Webhooks{}
}

// Messages is every message the fake accepted, in order.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.messages)
}

// Sign is the X-Twilio-Signature Twilio sends with a callback it posts to rawURL with form, signed
// with the account's Auth Token: the base64 HMAC-SHA1 of the URL followed by each POST parameter's
// name and value, sorted by name.
func Sign(authToken, rawURL string, form url.Values) string {
	var b strings.Builder
	b.WriteString(rawURL)
	names := make([]string, 0, len(form))
	for name := range form {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		for _, v := range form[name] {
			b.WriteString(name)
			b.WriteString(v)
		}
	}
	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fail(w, http.StatusBadRequest, 20001, "the form could not be read")
		return
	}
	form := r.Form
	if r.Method == http.MethodPost {
		form = r.PostForm
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	account, ok := s.authenticate(r)
	if !ok {
		fail(w, http.StatusUnauthorized, 20003, "Authenticate")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 4 && parts[0] == "2010-04-01" && parts[1] == "Accounts" && parts[3] == "IncomingPhoneNumbers.json" && r.Method == http.MethodGet:
		if parts[2] != account {
			fail(w, http.StatusUnauthorized, 20003, "Authenticate")
			return
		}
		s.listNumbers(w, account, form)
	case len(parts) == 4 && parts[0] == "2010-04-01" && parts[1] == "Accounts" && parts[3] == "Messages.json" && r.Method == http.MethodPost:
		if parts[2] != account {
			fail(w, http.StatusUnauthorized, 20003, "Authenticate")
			return
		}
		s.createMessage(w, account, form)
	case len(parts) == 5 && parts[0] == "2010-04-01" && parts[1] == "Accounts" && parts[3] == "IncomingPhoneNumbers" && r.Method == http.MethodPost:
		if parts[2] != account {
			fail(w, http.StatusNotFound, 20404, "The requested resource was not found")
			return
		}
		s.updateNumber(w, account, strings.TrimSuffix(parts[4], ".json"), form)
	case len(parts) == 3 && parts[0] == "v1" && parts[1] == "Services":
		s.service(w, r.Method, account, parts[2], form)
	default:
		fail(w, http.StatusNotFound, 20404, "The requested resource "+r.URL.Path+" was not found")
	}
}

// updateNumber sets the inbound webhook of the account's number of that SID.
func (s *Server) updateNumber(w http.ResponseWriter, account, numberSID string, form url.Values) {
	n, ok := s.numbers[numberSID]
	if !ok || n.account != account {
		fail(w, http.StatusNotFound, 20404, "The requested resource was not found")
		return
	}
	if s.forbidden[n.sid] {
		fail(w, http.StatusForbidden, 20403, "Forbidden")
		return
	}
	if v := form.Get("SmsUrl"); v != "" {
		n.webhooks.Inbound = v
	}
	answer(w, numberJSON(n))
}

// service reads the account's Messaging Service of that SID or, on a POST, sets its webhooks.
func (s *Server) service(w http.ResponseWriter, method, account, serviceSID string, form url.Values) {
	m, ok := s.services[serviceSID]
	if !ok || m.account != account {
		fail(w, http.StatusNotFound, 20404, "The requested resource /Services/"+serviceSID+" was not found")
		return
	}
	if method == http.MethodPost {
		if s.forbidden[m.sid] {
			fail(w, http.StatusForbidden, 20403, "Forbidden")
			return
		}
		m.webhooks.Inbound = form.Get("InboundRequestUrl")
		m.webhooks.Status = form.Get("StatusCallback")
		m.webhooks.UseInboundWebhookOnNumber = form.Get("UseInboundWebhookOnNumber")
	}
	answer(w, map[string]any{
		"sid": m.sid, "account_sid": m.account, "friendly_name": "Sender",
		"inbound_request_url": m.webhooks.Inbound, "status_callback": m.webhooks.Status,
	})
}

// authenticate is the account the call's credentials are of: the Account SID with its Auth Token,
// or an API key of the account with its secret.
func (s *Server) authenticate(r *http.Request) (string, bool) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	if a, ok := s.accounts[user]; ok && a.AuthToken == pass {
		return a.SID, true
	}
	if k, ok := s.keys[user]; ok && k.secret == pass {
		return k.account, true
	}
	return "", false
}

func (s *Server) listNumbers(w http.ResponseWriter, account string, form url.Values) {
	out := []map[string]any{}
	for _, n := range s.numbers {
		if n.account != account || (form.Get("PhoneNumber") != "" && form.Get("PhoneNumber") != n.number) {
			continue
		}
		out = append(out, numberJSON(n))
	}
	answer(w, map[string]any{"incoming_phone_numbers": out, "page": 0, "page_size": len(out), "next_page_uri": nil})
}

// createMessage accepts a message as Twilio does: to a number, from a number or an alphanumeric
// sender ID (From) or a Messaging Service of the account (MessagingServiceSid), with a body. A
// Messaging Service the account does not have is refused.
func (s *Server) createMessage(w http.ResponseWriter, account string, form url.Values) {
	to, body, from, service := form.Get("To"), form.Get("Body"), form.Get("From"), form.Get("MessagingServiceSid")
	switch {
	case to == "":
		fail(w, http.StatusBadRequest, 21604, "A 'To' phone number is required.")
		return
	case body == "":
		fail(w, http.StatusBadRequest, 21602, "Message body is required.")
		return
	case from == "" && service == "":
		fail(w, http.StatusBadRequest, 21603, "A 'From' or 'MessagingServiceSid' parameter is required to send a message.")
		return
	}
	if m, ok := s.services[service]; service != "" && (!ok || m.account != account) {
		fail(w, http.StatusBadRequest, 21701, "The Messaging Service Sid "+service+" is invalid.")
		return
	}
	if r := s.refusal; r != nil {
		if s.refuseOnce {
			s.refusal = nil
		}
		fail(w, r.Status, r.Code, r.Message)
		return
	}
	msg := Message{
		Account: account, SID: sid("SM"), To: to, From: from, MessagingServiceSID: service, Body: body,
		StatusCallback: form.Get("StatusCallback"),
	}
	s.messages = append(s.messages, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	cleanup.Report("write the fake Twilio's answer", json.NewEncoder(w).Encode(map[string]any{
		"sid": msg.SID, "account_sid": account, "to": to, "from": from, "messaging_service_sid": service,
		"body": body, "status": "queued", "num_segments": "1", "direction": "outbound-api",
	}))
}

func numberJSON(n *sender) map[string]any {
	return map[string]any{
		"sid": n.sid, "account_sid": n.account, "phone_number": n.number,
		"sms_url": n.webhooks.Inbound, "sms_method": "POST",
	}
}

func answer(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	cleanup.Report("write the fake Twilio's answer", json.NewEncoder(w).Encode(body))
}

// fail answers as Twilio refuses a call.
func fail(w http.ResponseWriter, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	cleanup.Report("write the fake Twilio's refusal", json.NewEncoder(w).Encode(map[string]any{
		"code": code, "message": message, "more_info": "https://www.twilio.com/docs/errors/" + strconv.Itoa(code), "status": status,
	}))
}
