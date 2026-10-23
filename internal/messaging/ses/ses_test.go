package ses_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sesAPI is an httptest stand-in for the SES v2 REST API; the provider's
// Config.Endpoint override points the real aws-sdk client at it.
type sesAPI struct {
	srv  *httptest.Server
	path string
	body map[string]any
	auth string
	hits int
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
		w.Header().Set("Content-Type", "application/json")
		if errorType != "" {
			w.Header().Set("X-Amzn-Errortype", errorType)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(api.srv.Close)
	return api
}

const okBody = `{"MessageId":"ses-msg-42"}`

const rejectedBody = `{"message":"identity not verified"}`

func throttledBody(message string) string {
	return `{"message":"` + message + `"}`
}

func build(t *testing.T, endpoint string) messaging.EmailSender {
	t.Helper()
	raw := fmt.Sprintf(`{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","fromName":"Acme","endpoint":%q}`, endpoint)
	built, err := ses.NewDescriptor(http.DefaultClient).Build([]byte(raw), nil)
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

	data := api.body["Content"].(map[string]any)["Raw"].(map[string]any)["Data"].(string)
	mime, err := base64.StdEncoding.DecodeString(data)
	require.NoError(t, err)
	assert.Contains(t, string(mime), "Subject: Hello")
	assert.Contains(t, string(mime), "\"Acme\" <noreply@acme.com>")
	assert.Contains(t, string(mime), "List-Unsubscribe: <https://x.test/u>", "custom headers survive in the raw message")
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

	t.Run("a Throttling reply is classified as too fast", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusTooManyRequests, "TooManyRequestsException", throttledBody("Maximum sending rate exceeded."))
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		require.ErrorIs(t, err, messaging.ErrBusy)
		assert.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("daily quota is classified as quota exceeded", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusTooManyRequests, "TooManyRequestsException", throttledBody("Daily message quota exceeded."))
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		assert.ErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("a rejected message is not busy", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusBadRequest, "MessageRejected", rejectedBody)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		require.NotErrorIs(t, err, messaging.ErrBusy)
		assert.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("unusable AWS environment config", func(t *testing.T) {
		t.Setenv("AWS_USE_FIPS_ENDPOINT", "not-a-bool")
		api := newSESAPI(t, http.StatusOK, okBody)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		require.ErrorContains(t, err, "ses: load config")
		assert.Zero(t, api.hits)
	})
}

func TestDefaultFrom(t *testing.T) {
	addr, name := build(t, "").(messaging.DefaultFromer).DefaultFrom()
	assert.Equal(t, "noreply@acme.com", addr)
	assert.Equal(t, "Acme", name)
}

func TestDescriptorValidate(t *testing.T) {
	d := ses.Descriptor()
	assert.Equal(t, messaging.ChannelEmail, d.Channel)
	assert.Equal(t, messaging.ProviderSES, d.Provider)

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
	_, err := ses.Descriptor().Build([]byte(`{`), nil)
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

	t.Run("a denied lookup is an error", func(t *testing.T) {
		api := newSESAPIWithErrorType(t, http.StatusForbidden, "AccessDeniedException", `{"message":"not authorized to perform ses:GetAccount"}`)
		_, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		assert.ErrorContains(t, err, "ses: get send quota")
	})
}

func TestSendRefusesPrivateEndpoint(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	raw := fmt.Sprintf(`{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","endpoint":%q}`, api.srv.URL)
	built, err := ses.Descriptor().Build([]byte(raw), nil)
	require.NoError(t, err)

	_, err = built.(messaging.EmailSender).Send(context.Background(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "Hello", Text: "body"})
	require.Error(t, err, "the production client must not dial a loopback endpoint")
	assert.Empty(t, api.auth, "no signed request reached the endpoint")
}
