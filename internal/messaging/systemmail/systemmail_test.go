package systemmail_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/systemmail"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// relayConfig is a system mail configuration whose SMTP relay is the sink.
func relayConfig(sink *testhelper.MailSink) *config.Config {
	return &config.Config{SMTPHost: sink.Host, SMTPPort: sink.Port, SystemEmailFrom: "noreply@example.test"}
}

// sendOne sends a message without a From of its own through sender and returns what the relay got.
func sendOne(t *testing.T, sender messaging.EmailSender, sink *testhelper.MailSink) testhelper.SunkMail {
	t.Helper()
	_, err := sender.Send(t.Context(), messaging.EmailMessage{To: "owner@example.test", Subject: "Notice", Text: "hello"})
	require.NoError(t, err)
	got := sink.Messages()
	require.Len(t, got, 1)
	return got[0]
}

func TestSenderMailsThroughTheSMTPRelayFromTheSystemAddress(t *testing.T) {
	sink := testhelper.NewMailSink(t)
	cfg := relayConfig(sink)
	cfg.SystemEmailFromName = "Example Mail"
	cfg.SystemEmailReplyTo = "replies@example.test"

	sender, err := systemmail.Sender(cfg, registry.Default())
	require.NoError(t, err)

	got := sendOne(t, sender, sink)
	from, err := mail.ParseAddress(got.Header.Get("From"))
	require.NoError(t, err)
	assert.Equal(t, mail.Address{Name: "Example Mail", Address: "noreply@example.test"}, *from)
	replyTo, err := mail.ParseAddress(got.Header.Get("Reply-To"))
	require.NoError(t, err)
	assert.Equal(t, "replies@example.test", replyTo.Address)
}

func TestSenderNamesTheSystemMailSphericonByDefault(t *testing.T) {
	sink := testhelper.NewMailSink(t)

	sender, err := systemmail.Sender(relayConfig(sink), registry.Default())
	require.NoError(t, err)

	from, err := mail.ParseAddress(sendOne(t, sender, sink).Header.Get("From"))
	require.NoError(t, err)
	assert.Equal(t, "sphericon", from.Name)
}

func TestSenderFallsBackToTheSMTPFromWhenNoSystemFromIsSet(t *testing.T) {
	sink := testhelper.NewMailSink(t)
	cfg := relayConfig(sink)
	cfg.SystemEmailFrom = ""
	cfg.SMTPFrom = "relay@example.test"

	sender, err := systemmail.Sender(cfg, registry.Default())
	require.NoError(t, err)

	from, err := mail.ParseAddress(sendOne(t, sender, sink).Header.Get("From"))
	require.NoError(t, err)
	assert.Equal(t, "relay@example.test", from.Address)
}

// Any provider name but "ses" is the SMTP relay, the documented default.
func TestSenderUsesTheSMTPRelayForAnyProviderButSES(t *testing.T) {
	sink := testhelper.NewMailSink(t)
	cfg := relayConfig(sink)
	cfg.SystemEmailProvider = "anything"

	sender, err := systemmail.Sender(cfg, registry.Default())
	require.NoError(t, err)

	assert.Equal(t, []string{"owner@example.test"}, sendOne(t, sender, sink).Rcpt)
}

// sesAPI stands in for the SES API the system sender reaches: it accepts every SendEmail and keeps
// the raw message it carried.
type sesAPI struct {
	mime []byte
}

func (a *sesAPI) Do(r *http.Request) (*http.Response, error) {
	var in struct {
		Content struct {
			Raw struct {
				Data []byte `json:"Data"`
			} `json:"Raw"`
		} `json:"Content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, err
	}
	a.mime = in.Content.Raw.Data
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"MessageId":"ses-1"}`)),
		Request:    r,
	}, nil
}

func TestSenderMailsThroughSESFromTheSystemAddress(t *testing.T) {
	api := &sesAPI{}
	cfg := &config.Config{
		SystemEmailProvider: "ses", SystemEmailFrom: "noreply@example.test", SystemEmailFromName: "Example Mail",
		SystemEmailReplyTo: "replies@example.test",
		SESRegion:          "eu-west-1", SESAccessKeyID: "AKIA", SESSecretAccessKey: "shh",
	}

	sender, err := systemmail.Sender(cfg, messaging.NewCatalog(ses.NewDescriptor(api)))
	require.NoError(t, err)
	_, err = sender.Send(t.Context(), messaging.EmailMessage{To: "owner@example.test", Subject: "Notice", Text: "hello"})
	require.NoError(t, err)

	msg, err := mail.ReadMessage(bytes.NewReader(api.mime))
	require.NoError(t, err)
	from, err := mail.ParseAddress(msg.Header.Get("From"))
	require.NoError(t, err)
	assert.Equal(t, mail.Address{Name: "Example Mail", Address: "noreply@example.test"}, *from)
	replyTo, err := mail.ParseAddress(msg.Header.Get("Reply-To"))
	require.NoError(t, err)
	assert.Equal(t, "replies@example.test", replyTo.Address)
}
