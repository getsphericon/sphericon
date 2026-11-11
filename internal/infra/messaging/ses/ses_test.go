package ses_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/dkimtest"
	"github.com/getsphericon/sphericon/internal/infra/messaging/ses"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sesAPI is an httptest stand-in for the SES v2 REST API; the provider's
// Config.Endpoint override points the real aws-sdk client at it.
type sesAPI struct {
	srv   *httptest.Server
	path  string
	body  map[string]any
	auth  string
	host  string
	token string
	hits  int
}

func newSESAPI(t *testing.T, status int, body string) *sesAPI {
	t.Helper()
	return newSESAPIWithErrorType(t, status, "", body)
}

// newSESAPIWithErrorType answers with the x-amzn-ErrorType header the v2 REST
// protocol uses to name an error.
func newSESAPIWithErrorType(t *testing.T, status int, errorType, body string) *sesAPI {
	t.Helper()
	api := &sesAPI{}
	api.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.hits++
		raw, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		api.body = nil
		if len(raw) > 0 {
			assert.NoError(t, json.Unmarshal(raw, &api.body))
		}
		api.path = r.URL.Path
		api.auth = r.Header.Get("Authorization")
		api.host = r.Host
		api.token = r.Header.Get("X-Amz-Security-Token")
		w.Header().Set("Content-Type", "application/json")
		if errorType != "" {
			w.Header().Set("X-Amzn-Errortype", errorType)
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(api.srv.Close)
	return api
}

// rawMessage is the MIME message of the last SendEmail call, decoded from its raw content.
func (a *sesAPI) rawMessage(t *testing.T) []byte {
	t.Helper()
	data := a.body["Content"].(map[string]any)["Raw"].(map[string]any)["Data"].(string)
	mime, err := base64.StdEncoding.DecodeString(data)
	require.NoError(t, err)
	return mime
}

const okBody = `{"MessageId":"ses-msg-42"}`

const rejectedBody = `{"message":"identity not verified"}`

func throttledBody(message string) string {
	return `{"message":"` + message + `"}`
}

func build(t *testing.T, endpoint string) messaging.EmailSender {
	t.Helper()
	return buildWithClient(t, endpoint, http.DefaultClient)
}

func buildWithClient(t *testing.T, endpoint string, client *http.Client) messaging.EmailSender {
	t.Helper()
	return buildWithDeps(t, endpoint, client, messaging.Deps{})
}

func buildWithDeps(t *testing.T, endpoint string, client *http.Client, deps messaging.Deps) messaging.EmailSender {
	t.Helper()
	raw := fmt.Sprintf(`{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","fromName":"Acme","endpoint":%q}`, endpoint)
	built, err := ses.NewDescriptor(client).Build([]byte(raw), deps)
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	return sender
}

func TestSendRawContent(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	sender := build(t, api.srv.URL)

	receipt, err := sender.Send(context.Background(), messaging.EmailMessage{
		To: "rcpt@example.com", Subject: "Hello", Text: "plain body", ListUnsubscribeURL: "https://x.test/u",
	})
	require.NoError(t, err)
	assert.Equal(t, "ses-msg-42", receipt.MessageID)

	assert.Equal(t, "/v2/email/outbound-emails", api.path)
	assert.Equal(t, "noreply@acme.com", api.body["FromEmailAddress"], "envelope sender falls back to the integration From")
	assert.Equal(t, []any{"rcpt@example.com"}, api.body["Destination"].(map[string]any)["ToAddresses"])
	assert.Contains(t, api.auth, "AKIATEST/", "request is SigV4-signed with the configured key")
	assert.Contains(t, api.auth, "/eu-west-1/ses/")

	mime := api.rawMessage(t)
	assert.Contains(t, string(mime), "Subject: Hello")
	assert.Contains(t, string(mime), "\"Acme\" <noreply@acme.com>")
	assert.Contains(t, string(mime), "List-Unsubscribe: <https://x.test/u>", "custom headers survive in the raw message")
}

// A Workspace's marketing message goes to SES as a DKIM-signed raw message, its one-click
// headers in the signed set, and the signature verifies on the bytes SES received.
func TestSendRawContentIsDKIMSigned(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	keys := dkimtest.New(t, "acme.com")
	sender := buildWithDeps(t, api.srv.URL, http.DefaultClient, messaging.Deps{DKIMKeys: keys})

	_, err := sender.Send(t.Context(), messaging.EmailMessage{
		To: "rcpt@example.com", Subject: "Hello", HTML: "<p>hi</p>", Text: "hi",
		ListUnsubscribeURL: "https://acme.com/e/u/tok",
	})
	require.NoError(t, err)

	mime := api.rawMessage(t)
	signed := keys.Verify(t, mime)
	assert.Subset(t, signed, []string{"From", "Subject", "List-Unsubscribe", "List-Unsubscribe-Post"})
}

func TestSendMessageFromOverridesDefault(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	sender := build(t, api.srv.URL)

	_, err := sender.Send(context.Background(), messaging.EmailMessage{
		From: "news@acme.com", To: "rcpt@example.com", Subject: "s", Text: "x",
	})
	require.NoError(t, err)
	assert.Equal(t, "news@acme.com", api.body["FromEmailAddress"])
}

func TestSendErrors(t *testing.T) {
	ctx := context.Background()
	msg := messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"}

	t.Run("invalid message never reaches the API", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, okBody)
		_, err := build(t, api.srv.URL).Send(ctx, messaging.EmailMessage{To: "bad", Text: "x"})
		require.ErrorContains(t, err, "invalid to address")
		assert.Zero(t, api.hits)
	})

	t.Run("API rejection is wrapped", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusBadRequest, "MessageRejected", rejectedBody)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		require.ErrorContains(t, err, "ses: send")
		assert.ErrorContains(t, err, "MessageRejected")
	})

	// SlowDownPlease is no code of SES's: the SDK counts its 429 as a throttle all the same.
	for _, code := range []string{"TooManyRequestsException", "LimitExceededException", "Throttling", "SlowDownPlease"} {
		t.Run(code+" is busy regardless of its message", func(t *testing.T) {
			api := newSESAPIWithErrorType(t, http.StatusTooManyRequests, code, throttledBody("Please try again later."))
			sender := build(t, api.srv.URL)
			_, err := sender.Send(ctx, msg)
			require.ErrorIs(t, err, messaging.ErrBusy)
			require.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
			assert.Equal(t, 1, api.hits, "throttling reaches Deferral without SDK retries")
		})
	}

	t.Run("daily quota is classified as quota exceeded", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusTooManyRequests, "TooManyRequestsException", throttledBody("Daily message quota exceeded."))
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		require.ErrorIs(t, err, messaging.ErrQuotaExceeded)
		require.NotErrorIs(t, err, messaging.ErrBusy)
		assert.Equal(t, 1, api.hits, "a spent daily quota is deferred without SDK retries")
	})

	t.Run("a refusal mentioning a rate limit is not busy", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusBadRequest, "MessageRejected", `{"message":"Message content mentions a rate limit."}`)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		require.ErrorIs(t, err, messaging.ErrRefused)
		require.NotErrorIs(t, err, messaging.ErrBusy)
		assert.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	})
}

func TestSendIgnoresHostAWSConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "invalid-config")
	err := os.WriteFile(configPath, []byte("[invalid\n"), 0o600)
	require.NoError(t, err)
	for key, value := range map[string]string{
		"AWS_USE_FIPS_ENDPOINT":       "not-a-bool",
		"AWS_RETRY_MODE":              "not-a-mode",
		"AWS_MAX_ATTEMPTS":            "not-a-number",
		"AWS_PROFILE":                 "missing-profile",
		"AWS_CONFIG_FILE":             configPath,
		"AWS_SHARED_CREDENTIALS_FILE": configPath,
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			api := newSESAPI(t, http.StatusOK, okBody)
			sender := build(t, api.srv.URL)
			receipt, err := sender.Send(t.Context(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"})
			require.NoError(t, err)
			assert.Equal(t, "ses-msg-42", receipt.MessageID)
		})
	}
}

func TestSendIgnoresHostAWSEndpointAndCredentials(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "HOSTKEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "host-secret")
	t.Setenv("AWS_SESSION_TOKEN", "host-token")
	for _, key := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_SESV2"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "https://host-config.example")
			api := newSESAPI(t, http.StatusOK, okBody)
			sender := buildWithClient(t, "", &http.Client{Transport: toServer{target: api.srv.URL}})
			receipt, err := sender.Send(t.Context(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"})
			require.NoError(t, err)
			assert.Equal(t, "ses-msg-42", receipt.MessageID)
			assert.Equal(t, "email.eu-west-1.amazonaws.com", api.host)
			assert.Contains(t, api.auth, "AKIATEST/")
			assert.Contains(t, api.auth, "/eu-west-1/ses/")
			assert.Empty(t, api.token)
		})
	}
}

func TestSendRetriesServerErrors(t *testing.T) {
	api := newSESAPIWithErrorType(t, http.StatusServiceUnavailable, "ServiceUnavailableException", `{"message":"temporarily unavailable"}`)
	sender := build(t, api.srv.URL)
	_, err := sender.Send(t.Context(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"})
	require.Error(t, err)
	require.NotErrorIs(t, err, messaging.ErrBusy)
	require.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	assert.Equal(t, 3, api.hits, "transient server errors keep the SDK's standard retry budget")
}

func TestDefaultFrom(t *testing.T) {
	addr, name := build(t, "").(messaging.DefaultFromer).DefaultFrom()
	assert.Equal(t, "noreply@acme.com", addr)
	assert.Equal(t, "Acme", name)
}

func TestDescriptorValidate(t *testing.T) {
	d := ses.Descriptor()
	assert.Equal(t, channels.Email, d.Channel)
	assert.Equal(t, providers.SES, d.Provider)

	cases := map[string]struct {
		cfg     string
		wantErr string
	}{
		"valid":          {`{"region":"r","accessKeyId":"k","from":"a@b.com"}`, ""},
		"malformed":      {`{`, "invalid config"},
		"missing region": {`{"accessKeyId":"k","from":"a@b.com"}`, "region is required"},
		"missing key":    {`{"region":"r","from":"a@b.com"}`, "accessKeyId is required"},
		"missing from":   {`{"region":"r","accessKeyId":"k"}`, "from is required"},
		"https endpoint": {`{"region":"r","accessKeyId":"k","from":"a@b.com","endpoint":"https://postbox.cloud.yandex.net"}`, ""},
		"http endpoint":  {`{"region":"r","accessKeyId":"k","from":"a@b.com","endpoint":"http://attacker.example"}`, "endpoint must be an https URL"},
		"no host":        {`{"region":"r","accessKeyId":"k","from":"a@b.com","endpoint":"https://"}`, "endpoint must be an https URL"},
		"bare host":      {`{"region":"r","accessKeyId":"k","from":"a@b.com","endpoint":"attacker.example"}`, "endpoint must be an https URL"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := d.Validate([]byte(tc.cfg))
			testhelper.ErrorContainsOrNone(t, err, tc.wantErr)
		})
	}
}

func TestBuildRejectsMalformedConfig(t *testing.T) {
	_, err := ses.Descriptor().Build([]byte(`{`), messaging.Deps{})
	assert.ErrorContains(t, err, "invalid config")
}

func quotaBody(max24h, rate string) string {
	return `{"SendQuota":{"Max24HourSend":` + max24h + `,"MaxSendRate":` + rate + `,"SentLast24Hours":7.0}}`
}

func quotaReader(t *testing.T, endpoint string) messaging.QuotaReader {
	t.Helper()
	reader, ok := build(t, endpoint).(messaging.QuotaReader)
	require.True(t, ok, "the SES sender reports the account's send quota")
	return reader
}

func accountBody(enforcement string, productionAccess bool) string {
	return fmt.Sprintf(`{"SendQuota":{"Max24HourSend":50000.0,"MaxSendRate":14.0,"SentLast24Hours":7.0},"EnforcementStatus":%q,"ProductionAccessEnabled":%t}`, enforcement, productionAccess)
}

// toServer sends every request to the server at target, whatever host it was addressed to, so
// a sender with no endpoint of its own (AWS) reaches the stand-in.
type toServer struct{ target string }

func (r toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(r.target)
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	req.Host = req.URL.Host
	req.URL.Scheme, req.URL.Host = target.Scheme, target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// awsQuotaReader is the quota reader of an Integration on AWS itself (no endpoint), its
// requests answered by the stand-in at serverURL.
func awsQuotaReader(t *testing.T, serverURL string) messaging.QuotaReader {
	t.Helper()
	built := buildWithClient(t, "", &http.Client{Transport: toServer{target: serverURL}})
	reader, ok := built.(messaging.QuotaReader)
	require.True(t, ok)
	return reader
}

func TestSendQuota(t *testing.T) {
	ctx := context.Background()

	t.Run("reads the account's rate and daily quota", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, quotaBody("50000.0", "14.0"))
		got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.Equal(t, "/v2/email/account", api.path)
		require.NotNil(t, got.PerSecond)
		require.NotNil(t, got.PerDay)
		assert.Equal(t, 14, *got.PerSecond)
		assert.Equal(t, 50000, *got.PerDay)
	})

	t.Run("a fractional rate rounds down but never to zero", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, quotaBody("200.0", "0.5"))
		got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, *got.PerSecond, "a sandbox account sends one message per second")
	})

	t.Run("a value the provider does not bound is no ceiling", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, quotaBody("-1.0", "0.0"))
		got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.Nil(t, got.PerSecond)
		assert.Nil(t, got.PerDay)
	})

	t.Run("a healthy account with production access reports nothing to flag", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, accountBody("HEALTHY", true))
		got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.Equal(t, providers.EnforcementNone, got.Enforcement)
		assert.False(t, got.Sandbox)
	})

	t.Run("reads the enforcement status", func(t *testing.T) {
		for status, want := range map[string]providers.Enforcement{
			"PROBATION": providers.EnforcementProbation,
			"SHUTDOWN":  providers.EnforcementShutdown,
			"HEALTHY":   providers.EnforcementNone,
			"SOMETHING": providers.EnforcementNone,
		} {
			api := newSESAPI(t, http.StatusOK, accountBody(status, true))
			got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
			require.NoError(t, err)
			assert.Equal(t, want, got.Enforcement, status)
		}
	})

	t.Run("an answer without an enforcement status is no enforcement", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, quotaBody("200.0", "1.0"))
		got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.Equal(t, providers.EnforcementNone, got.Enforcement)
	})

	t.Run("an AWS account without production access is in the sandbox", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, accountBody("HEALTHY", false))
		got, err := awsQuotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.Equal(t, "/v2/email/account", api.path)
		assert.True(t, got.Sandbox)
	})

	t.Run("an AWS account with production access is not in the sandbox", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, accountBody("HEALTHY", true))
		got, err := awsQuotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.False(t, got.Sandbox)
	})

	t.Run("an SES-compatible service has no sandbox, whatever it answers", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, quotaBody("200.0", "1.0"))
		got, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		require.NoError(t, err)
		assert.False(t, got.Sandbox, "an answer without ProductionAccessEnabled is no sandbox")
	})

	t.Run("a denied lookup is an error", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusForbidden, "AccessDeniedException", `{"message":"not authorized to perform ses:GetAccount"}`)
		_, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		assert.ErrorContains(t, err, "ses: get send quota")
	})
}

func TestSendRefusesPrivateEndpoint(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	raw := fmt.Sprintf(`{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","endpoint":%q}`, api.srv.URL)
	built, err := ses.Descriptor().Build([]byte(raw), messaging.Deps{})
	require.NoError(t, err)

	_, err = built.(messaging.EmailSender).Send(context.Background(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "Hello", Text: "body"})
	require.Error(t, err, "the production client must not dial a loopback endpoint")
	assert.Empty(t, api.auth, "no signed request reached the endpoint")
}

func TestSendPutsTheConfiguredReplyToOnTheMessage(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	raw := fmt.Sprintf(`{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","replyTo":"help@acme.com","endpoint":%q}`, api.srv.URL)
	built, err := ses.NewDescriptor(http.DefaultClient).Build([]byte(raw), messaging.Deps{})
	require.NoError(t, err)

	_, err = built.(messaging.EmailSender).Send(context.Background(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"})
	require.NoError(t, err)

	mime := api.rawMessage(t)
	assert.Contains(t, string(mime), "Reply-To: <help@acme.com>")
}
