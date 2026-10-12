package ses_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
)

// sesAPI is an httptest stand-in for the SES query API; the provider's
// Config.Endpoint override points the real aws-sdk client at it.
type sesAPI struct {
	srv  *httptest.Server
	form url.Values
	auth string
	hits int
}

func newSESAPI(t *testing.T, status int, body string) *sesAPI {
	t.Helper()
	api := &sesAPI{}
	api.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.hits++
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		api.form, err = url.ParseQuery(string(raw))
		require.NoError(t, err)
		api.auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(api.srv.Close)
	return api
}

const okBody = `<SendRawEmailResponse xmlns="http://ses.amazonaws.com/doc/2010-12-01/">
<SendRawEmailResult><MessageId>ses-msg-42</MessageId></SendRawEmailResult>
<ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata></SendRawEmailResponse>`

const rejectedBody = `<ErrorResponse xmlns="http://ses.amazonaws.com/doc/2010-12-01/">
<Error><Type>Sender</Type><Code>MessageRejected</Code><Message>identity not verified</Message></Error>
<RequestId>req-2</RequestId></ErrorResponse>`

func throttledBody(message string) string {
	return `<ErrorResponse xmlns="http://ses.amazonaws.com/doc/2010-12-01/">
<Error><Type>Sender</Type><Code>Throttling</Code><Message>` + message + `</Message></Error>
<RequestId>req-3</RequestId></ErrorResponse>`
}

func build(t *testing.T, endpoint string) messaging.EmailSender {
	t.Helper()
	raw := fmt.Sprintf(`{"region":"eu-west-1","accessKeyId":"AKIATEST","secretAccessKey":"shh","from":"noreply@acme.com","fromName":"Acme","endpoint":%q}`, endpoint)
	built, err := ses.Descriptor().Build([]byte(raw), nil)
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	return sender
}

func TestSendRawEmail(t *testing.T) {
	api := newSESAPI(t, http.StatusOK, okBody)
	sender := build(t, api.srv.URL)

	receipt, err := sender.Send(context.Background(), messaging.EmailMessage{
		To: "rcpt@example.com", Subject: "Hello", Text: "plain body", ListUnsubscribeURL: "https://x.test/u",
	})
	require.NoError(t, err)
	assert.Equal(t, "ses-msg-42", receipt.MessageID)

	assert.Equal(t, "SendRawEmail", api.form.Get("Action"))
	assert.Equal(t, "noreply@acme.com", api.form.Get("Source"), "envelope Source falls back to the integration From")
	assert.Equal(t, "rcpt@example.com", api.form.Get("Destinations.member.1"))
	assert.Contains(t, api.auth, "AKIATEST/", "request is SigV4-signed with the configured key")
	assert.Contains(t, api.auth, "/eu-west-1/ses/")

	mime, err := base64.StdEncoding.DecodeString(api.form.Get("RawMessage.Data"))
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
	assert.Equal(t, "news@acme.com", api.form.Get("Source"))
}

func TestSendErrors(t *testing.T) {
	ctx := context.Background()
	msg := messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"}

	t.Run("invalid message never reaches the API", func(t *testing.T) {
		api := newSESAPI(t, http.StatusOK, okBody)
		_, err := build(t, api.srv.URL).Send(ctx, messaging.EmailMessage{To: "bad", Text: "x"})
		assert.ErrorContains(t, err, "invalid to address")
		assert.Zero(t, api.hits)
	})

	t.Run("API rejection is wrapped", func(t *testing.T) {
		api := newSESAPI(t, http.StatusBadRequest, rejectedBody)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		assert.ErrorContains(t, err, "ses: send")
		assert.ErrorContains(t, err, "MessageRejected")
	})

	t.Run("a Throttling reply is classified as too fast", func(t *testing.T) {
		api := newSESAPI(t, http.StatusBadRequest, throttledBody("Maximum sending rate exceeded."))
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		assert.ErrorIs(t, err, messaging.ErrBusy)
		assert.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("daily quota is classified as quota exceeded", func(t *testing.T) {
		api := newSESAPI(t, http.StatusBadRequest, throttledBody("Daily message quota exceeded."))
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		assert.ErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("a rejected message is not busy", func(t *testing.T) {
		api := newSESAPI(t, http.StatusBadRequest, rejectedBody)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		assert.NotErrorIs(t, err, messaging.ErrBusy)
		assert.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("unusable AWS environment config", func(t *testing.T) {
		t.Setenv("AWS_USE_FIPS_ENDPOINT", "not-a-bool")
		api := newSESAPI(t, http.StatusOK, okBody)
		_, err := build(t, api.srv.URL).Send(ctx, msg)
		assert.ErrorContains(t, err, "ses: load config")
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
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := d.Validate([]byte(tc.cfg))
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestBuildRejectsMalformedConfig(t *testing.T) {
	_, err := ses.Descriptor().Build([]byte(`{`), nil)
	assert.ErrorContains(t, err, "invalid config")
}

func quotaBody(max24h, rate string) string {
	return `<GetSendQuotaResponse xmlns="http://ses.amazonaws.com/doc/2010-12-01/">
<GetSendQuotaResult><Max24HourSend>` + max24h + `</Max24HourSend><MaxSendRate>` + rate + `</MaxSendRate><SentLast24Hours>7.0</SentLast24Hours></GetSendQuotaResult>
<ResponseMetadata><RequestId>req-4</RequestId></ResponseMetadata></GetSendQuotaResponse>`
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
		assert.Equal(t, "GetSendQuota", api.form.Get("Action"))
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
		api := newSESAPI(t, http.StatusForbidden, `<ErrorResponse xmlns="http://ses.amazonaws.com/doc/2010-12-01/">
<Error><Type>Sender</Type><Code>AccessDenied</Code><Message>not authorized to perform ses:GetSendQuota</Message></Error>
<RequestId>req-5</RequestId></ErrorResponse>`)
		_, err := quotaReader(t, api.srv.URL).SendQuota(ctx)
		assert.ErrorContains(t, err, "ses: get send quota")
	})
}
