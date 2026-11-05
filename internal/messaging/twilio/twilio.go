// Package twilio implements the SMS Channel's provider: a Workspace's own Twilio account, reached
// through Twilio's REST API with github.com/twilio/twilio-go at the Instance's Twilio API base URL
// (SPHERICON_TWILIO_API_URL). Its config is the send credential (an API key and its secret, or the
// Account SID and its Auth Token) and what messages go out from. The Auth Token that signs Twilio's
// callbacks is not here: it is the `twilio` Preset's param, kept and rotated apart from this.
package twilio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	twclient "github.com/twilio/twilio-go/client"
	api "github.com/twilio/twilio-go/rest/api/v2010"
	msgapi "github.com/twilio/twilio-go/rest/messaging/v1"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// DefaultAPIURL is Twilio's own REST API.
const DefaultAPIURL = "https://api.twilio.com"

// MaxLength is the longest text Twilio sends, in characters as smsrender.Measure counts them:
// 1,600 in GSM-7 and 700 in UCS-2, the maximum length of a concatenated message
// (https://www.twilio.com/docs/glossary/what-sms-character-limit). Twilio refuses a longer body
// with error 21617 (https://www.twilio.com/docs/api/errors/21617) and documents no cap on segments.
// A GSM-7 extension character counts two and a UCS-2 character beyond the BMP two UTF-16 code
// units, as they are encoded, so a text within the limit always goes.
var MaxLength = smsrender.Limit{GSM7: 1600, UCS2: 700}

// timeout bounds one REST call. twilio-go builds its requests without the caller's context, so the
// client below carries the context in, with this deadline: it also bounds a call whose context has
// none.
const timeout = 15 * time.Second

// maxRefusalBody is the most of a refusal's body read: Twilio's error JSON is a few hundred bytes.
const maxRefusalBody = 64 << 10

// Config is the stored config of a Twilio Integration. Secret is the API key's secret when
// APIKeySID is set, else the account's Auth Token. From is a number in E.164, an alphanumeric
// sender ID or a Messaging Service SID.
type Config struct {
	AccountSID string `json:"account_sid"`
	APIKeySID  string `json:"api_key_sid,omitempty"`
	Secret     string `json:"secret"`
	From       string `json:"from"`
	// CallbacksSet records that connecting pointed the sender's callbacks at the Integration's hook;
	// false when they could not be set and the address is to be given to Twilio by hand.
	CallbacksSet bool `json:"callbacks_set,omitempty"`
}

var (
	accountSIDFormat = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	apiKeySIDFormat  = regexp.MustCompile(`^SK[0-9a-fA-F]{32}$`)
	serviceSIDFormat = regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`)
	numberFormat     = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
	// An alphanumeric sender ID is up to 11 letters, digits and spaces, with at least one letter.
	alphanumericFormat = regexp.MustCompile(`^[A-Za-z0-9 ]{1,11}$`)
	hasLetter          = regexp.MustCompile(`[A-Za-z]`)
	// secretFormat is what twilio-go sends as a password: letters and digits only.
	secretFormat = regexp.MustCompile(`^[A-Za-z0-9]+$`)
)

// Field errors of a config, each naming the field it is about.
var (
	ErrAccountSID = errors.New("this is not a Twilio Account SID: it starts with AC and has 34 characters")
	ErrAPIKeySID  = errors.New("this is not a Twilio API key SID: it starts with SK and has 34 characters")
	ErrSecret     = errors.New("the secret is letters and digits, as Twilio shows it")
	ErrFrom       = errors.New("send from a phone number in international format (+15017250604), an alphanumeric sender ID of up to 11 characters or a Messaging Service SID (MG…)")
)

// FieldError is a config refusal on one of its fields: accountSid, apiKeySid, secret or from.
type FieldError struct {
	Field string
	Err   error
}

func (e *FieldError) Error() string { return e.Err.Error() }
func (e *FieldError) Unwrap() error { return e.Err }

// FromKind is what from names, ok false when it names nothing Twilio sends from.
func FromKind(from string) (messaging.SMSFromKind, bool) {
	switch {
	case serviceSIDFormat.MatchString(from):
		return messaging.SMSFromMessagingService, true
	case numberFormat.MatchString(from):
		return messaging.SMSFromNumber, true
	case alphanumericFormat.MatchString(from) && hasLetter.MatchString(from):
		return messaging.SMSFromAlphanumeric, true
	}
	return "", false
}

// Validate checks the shape of a config: the SIDs, the secret and the sender. A blank secret is
// refused: the caller carries a stored one forward before validating.
func Validate(c Config) error {
	if !accountSIDFormat.MatchString(c.AccountSID) {
		return &FieldError{Field: "accountSid", Err: ErrAccountSID}
	}
	if c.APIKeySID != "" && !apiKeySIDFormat.MatchString(c.APIKeySID) {
		return &FieldError{Field: "apiKeySid", Err: ErrAPIKeySID}
	}
	if !secretFormat.MatchString(c.Secret) {
		return &FieldError{Field: "secret", Err: ErrSecret}
	}
	if _, ok := FromKind(c.From); !ok {
		return &FieldError{Field: "from", Err: ErrFrom}
	}
	return nil
}

// Descriptor registers the Twilio provider with the messaging catalog; apiURL is the REST API base
// URL every account of the Instance is reached at.
func Descriptor(apiURL string) messaging.ProviderDescriptor {
	return messaging.ProviderDescriptor{
		Channel:  channels.SMS,
		Provider: providers.Twilio,
		Validate: func(config []byte) error {
			c, err := parse(config)
			if err != nil {
				return err
			}
			return Validate(c)
		},
		Build: func(config []byte, _ messaging.Deps) (any, error) {
			c, err := parse(config)
			if err != nil {
				return nil, err
			}
			return New(apiURL, c)
		},
	}
}

func parse(config []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(config, &c); err != nil {
		return Config{}, fmt.Errorf("twilio: read config: %w", err)
	}
	return c, nil
}

// Account is one Workspace's Twilio account, sending from its configured sender.
type Account struct {
	base *url.URL
	cfg  Config
}

// New builds the account of cfg at the REST API base URL apiURL.
func New(apiURL string, cfg Config) (*Account, error) {
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	base, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("twilio: the API URL %q is not a URL", apiURL)
	}
	return &Account{base: base, cfg: cfg}, nil
}

// Sender reads the configured sender: the account's number of that value, its Messaging Service of
// that SID, or for an alphanumeric sender ID one page of the account's numbers, which only checks
// the credentials. A Standard API key may not read the Account resource itself, so the read is of
// what the key is used for.
func (a *Account) Sender(ctx context.Context) (messaging.SMSFrom, error) {
	kind, _ := FromKind(a.cfg.From)
	switch kind {
	case messaging.SMSFromMessagingService:
		svc, err := a.messaging(ctx).FetchService(a.cfg.From)
		if err != nil {
			return messaging.SMSFrom{}, a.refusal(err)
		}
		return messaging.SMSFrom{Kind: kind, SID: deref(svc.Sid)}, nil
	case messaging.SMSFromNumber:
		page, err := a.api(ctx).PageIncomingPhoneNumber((&api.ListIncomingPhoneNumberParams{}).
			SetPathAccountSid(a.cfg.AccountSID).SetPhoneNumber(a.cfg.From).SetPageSize(20), "", "")
		if err != nil {
			return messaging.SMSFrom{}, a.refusal(err)
		}
		for i := range page.IncomingPhoneNumbers {
			if n := &page.IncomingPhoneNumbers[i]; deref(n.PhoneNumber) == a.cfg.From {
				return messaging.SMSFrom{Kind: kind, SID: deref(n.Sid)}, nil
			}
		}
		return messaging.SMSFrom{}, &messaging.SMSError{Status: http.StatusNotFound, Message: "the account has no number " + a.cfg.From}
	default:
		_, err := a.api(ctx).PageIncomingPhoneNumber((&api.ListIncomingPhoneNumberParams{}).
			SetPathAccountSid(a.cfg.AccountSID).SetPageSize(1), "", "")
		if err != nil {
			return messaging.SMSFrom{}, a.refusal(err)
		}
		return messaging.SMSFrom{Kind: messaging.SMSFromAlphanumeric}, nil
	}
}

// PointCallbacks sets where Twilio posts: a number's inbound messages (SmsUrl; its delivery reports
// are asked for with each message), or a Messaging Service's inbound messages and delivery reports,
// overriding the webhooks of its numbers. An alphanumeric sender ID receives nothing and keeps no
// callback. Credentials that may not change the sender leave it as it is and report false.
func (a *Account) PointCallbacks(ctx context.Context, from messaging.SMSFrom, hook string) (bool, error) {
	var err error
	switch from.Kind {
	case messaging.SMSFromNumber:
		_, err = a.api(ctx).UpdateIncomingPhoneNumber(from.SID, (&api.UpdateIncomingPhoneNumberParams{}).
			SetPathAccountSid(a.cfg.AccountSID).SetSmsUrl(hook).SetSmsMethod(http.MethodPost))
	case messaging.SMSFromMessagingService:
		_, err = a.messaging(ctx).UpdateService(from.SID, (&msgapi.UpdateServiceParams{}).
			SetInboundRequestUrl(hook).SetInboundMethod(http.MethodPost).SetStatusCallback(hook).
			SetUseInboundWebhookOnNumber(false))
	default:
		return false, nil
	}
	if err == nil {
		return true, nil
	}
	refused := a.refusal(err)
	if e, ok := errors.AsType[*messaging.SMSError](refused); ok && e.Status == http.StatusForbidden {
		return false, nil
	}
	return false, refused
}

// Send creates the message: from the configured number or alphanumeric sender ID, or through the
// configured Messaging Service, which picks the number. It always names the status callback: a
// number keeps none for messages, and the one a Messaging Service keeps may point elsewhere.
func (a *Account) Send(ctx context.Context, msg messaging.SMSMessage) (messaging.SMSSent, error) {
	params := (&api.CreateMessageParams{}).SetPathAccountSid(a.cfg.AccountSID).
		SetTo(msg.To).SetBody(msg.Body).SetStatusCallback(msg.StatusCallback)
	if kind, _ := FromKind(a.cfg.From); kind == messaging.SMSFromMessagingService {
		params.SetMessagingServiceSid(a.cfg.From)
	} else {
		params.SetFrom(a.cfg.From)
	}
	created, err := a.api(ctx).CreateMessage(params)
	if err != nil {
		return messaging.SMSSent{}, a.refusal(err)
	}
	return messaging.SMSSent{ID: deref(created.Sid)}, nil
}

// api is the 2010-04-01 REST API of the account, its calls bound to ctx.
func (a *Account) api(ctx context.Context) *api.ApiService {
	return api.NewApiService(&twclient.RequestHandler{Client: a.client(ctx)})
}

// messaging is the Messaging Services API of the account, its calls bound to ctx.
func (a *Account) messaging(ctx context.Context) *msgapi.ApiService {
	return msgapi.NewApiService(&twclient.RequestHandler{Client: a.client(ctx)})
}

// client is twilio-go's client with the account's credentials, reaching the Instance's base URL
// with ctx. The RequestHandler is built without twilio-go's constructor, which would take an edge
// and a region from the process's environment and rewrite the host with them.
func (a *Account) client(ctx context.Context) twclient.BaseClient {
	user := a.cfg.AccountSID
	if a.cfg.APIKeySID != "" {
		user = a.cfg.APIKeySID
	}
	c := &twclient.Client{
		Credentials: twclient.NewCredentials(user, a.cfg.Secret),
		HTTPClient: &http.Client{
			// No Timeout: the transport bounds each call (withContext), which a client-wide timeout
			// would duplicate with a timer that waits for a body twilio-go never closes.
			Transport: withContext(ctx),
			// As twilio-go's own client: an answer is never followed elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	c.SetAccountSid(a.cfg.AccountSID)
	return rebased{Client: c, base: a.base}
}

// rebased sends every call to the Instance's base URL: the API's own host becomes the base's, and
// another Twilio API (messaging) the base's host with its first label swapped when the base is an
// `api.` host, else the base's host too.
type rebased struct {
	*twclient.Client

	base *url.URL
}

func (r rebased) SendRequest(method, rawURL string, data url.Values, headers map[string]any, body ...byte) (*http.Response, error) {
	target, err := rebase(r.base, rawURL)
	if err != nil {
		return nil, err
	}
	return r.Client.SendRequest(method, target, data, headers, body...)
}

// rebase is where a call twilio-go addresses to rawURL (on one of Twilio's own hosts) goes at base.
func rebase(base *url.URL, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := base.Host
	if product, _, _ := strings.Cut(u.Host, "."); product != "api" && strings.HasPrefix(host, "api.") {
		host = product + strings.TrimPrefix(host, "api")
	}
	u.Scheme, u.Host, u.Path = base.Scheme, host, strings.TrimRight(base.Path, "/")+u.Path
	return u.String(), nil
}

// withContext is a transport that sends each request with the caller's context, which twilio-go
// does not pass on, bounded by timeout. twilio-go decodes a refusal's body and never closes it, so a
// refusal's body is read whole here and its connection released; any other answer's body releases
// the call when twilio-go closes it.
func withContext(ctx context.Context) http.RoundTripper {
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		res, err := http.DefaultTransport.RoundTrip(req.WithContext(ctx))
		if err != nil {
			cancel()
			return nil, err
		}
		if res.StatusCode >= http.StatusBadRequest {
			defer cancel()
			body, err := io.ReadAll(io.LimitReader(res.Body, maxRefusalBody))
			cleanup.Report("close a Twilio refusal's body", res.Body.Close())
			if err != nil {
				return nil, err
			}
			res.Body = io.NopCloser(bytes.NewReader(body))
			return res, nil
		}
		res.Body = cancelOnClose{ReadCloser: res.Body, cancel: cancel}
		return res, nil
	})
}

// cancelOnClose is a response body that ends its call's context once it is closed.
type cancelOnClose struct {
	io.ReadCloser

	cancel context.CancelFunc
}

func (b cancelOnClose) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}

// roundTripper is a function as an http.RoundTripper.
type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// refusal turns twilio-go's error into a *messaging.SMSError when Twilio answered one, sorted by
// Classify. Any other error (the network, an answer that is not Twilio's) is returned with the
// secret taken out of its text, its chain kept for errors.Is and errors.As.
func (a *Account) refusal(err error) error {
	if rest, ok := errors.AsType[*twclient.TwilioRestError](err); ok {
		return &messaging.SMSError{Status: rest.Status, Code: rest.Code, Message: rest.Message, Refusal: Classify(rest.Status, rest.Code, a.fromKind())}
	}
	if rest, ok := errors.AsType[*twclient.RestErrorV1](err); ok {
		return &messaging.SMSError{Status: rest.HttpStatusCode, Code: rest.Code, Message: rest.Message, Refusal: Classify(rest.HttpStatusCode, rest.Code, a.fromKind())}
	}
	return redacted{msg: strings.ReplaceAll(err.Error(), a.cfg.Secret, "<secret>"), cause: err}
}

// fromKind is the kind of sender the account sends from.
func (a *Account) fromKind() messaging.SMSFromKind {
	kind, _ := FromKind(a.cfg.From)
	return kind
}

// redacted is an error whose text has the secret removed and whose cause is kept for Is and As.
type redacted struct {
	msg   string
	cause error
}

func (e redacted) Error() string { return e.msg }
func (e redacted) Unwrap() error { return e.cause }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// codeFromNotCapable is Twilio's 21606: the From cannot send to the To.
const codeFromNotCapable = 21606

// The error-code table: whom each of Twilio's error codes concerns (ADR 0079). Each entry is
// Twilio's own description at https://www.twilio.com/docs/api/errors/<code>. A code the table does
// not name is ambiguous: it is recorded on the message and changes nothing else.
var refusals = map[int]messaging.SMSRefusal{
	// The account. 20003: the credentials are refused (wrong, revoked, of a suspended or closed
	// account). 20005: the account is not active (suspended, an unactivated trial). 21212: the From
	// is not a number, sender ID or sender Twilio recognises. 21606: the From is not a
	// message-capable number of this account. 21659: the From is not a Twilio number. 21660: the
	// From belongs to another account than the credentials. 21701: the Messaging Service does not
	// exist on the account. 21704: the Messaging Service has no sender to send from.
	20003: messaging.SMSRefusalAccount,
	20005: messaging.SMSRefusalAccount,
	21212: messaging.SMSRefusalAccount,
	21606: messaging.SMSRefusalAccount,
	21659: messaging.SMSRefusalAccount,
	21660: messaging.SMSRefusalAccount,
	21701: messaging.SMSRefusalAccount,
	21704: messaging.SMSRefusalAccount,

	// The rate. 20429: too many concurrent REST calls, "safe to retry after backing off". 21611:
	// the From's queue is full. 14107: more than 30 messages between the same two numbers in 30
	// seconds.
	20429: messaging.SMSRefusalRate,
	21611: messaging.SMSRefusalRate,
	14107: messaging.SMSRefusalRate,

	// The number. 21211: the To is not a valid phone number. 21408: the To's region is not
	// permitted for the account (Geo Permissions), which concerns that region's numbers only.
	// 21608: a trial account may text verified numbers only. 21612: the To cannot be reached from
	// this sender (the carrier or the country refuses it).
	21211: messaging.SMSRefusalDestination,
	21408: messaging.SMSRefusalDestination,
	21608: messaging.SMSRefusalDestination,
	21612: messaging.SMSRefusalDestination,

	// The person. 21610: "Attempt to send to unsubscribed recipient": the To texted STOP to this
	// sender, and Twilio refuses every message from it until the To texts START.
	21610: messaging.SMSRefusalOptedOut,

	// A number that reaches no handset. 21614 (at send time): the To is not a valid mobile number,
	// a landline. 30005 and 30006 (in a delivery report): an unknown handset, no longer in service,
	// and a landline or an unreachable carrier.
	21614: messaging.SMSRefusalDeadNumber,
	30005: messaging.SMSRefusalDeadNumber,
	30006: messaging.SMSRefusalDeadNumber,

	// Ambiguous, named so the table says so (in a delivery report). 30003: the handset is
	// unreachable (off, out of coverage). 30007: a carrier filter. 30008: an unknown error.
	30003: messaging.SMSRefusalAmbiguous,
	30007: messaging.SMSRefusalAmbiguous,
	30008: messaging.SMSRefusalAmbiguous,
}

// Classify is whom Twilio's answer of HTTP status and error code concerns, for a message sent from
// a sender of kind from ("" when it is not known, as for a delivery report). The status decides
// first, whatever the code: a 401 or a 403 is the account's, a 429 the rate's and a 5xx Twilio's
// own failure (transient). Otherwise the code decides by the table, ambiguous when it is not there,
// except 21606 from an alphanumeric sender ID: a sender ID has no number to be message-capable, so
// the refusal is the To's country not taking sender IDs, which concerns that number alone.
func Classify(status, code int, from messaging.SMSFromKind) messaging.SMSRefusal {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return messaging.SMSRefusalAccount
	case status == http.StatusTooManyRequests:
		return messaging.SMSRefusalRate
	case status >= http.StatusInternalServerError:
		return messaging.SMSRefusalTransient
	}
	if code == codeFromNotCapable && from == messaging.SMSFromAlphanumeric {
		return messaging.SMSRefusalDestination
	}
	if r, ok := refusals[code]; ok {
		return r
	}
	return messaging.SMSRefusalAmbiguous
}
