package smtp_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
)

// capture is one message accepted by the test server.
type capture struct {
	from, rcpt, data string
	// username and password are what the client authenticated with, empty when it did not.
	username, password string
}

// testServer is a go-smtp server on a loopback port. It never offers STARTTLS,
// like a local relay, and records every accepted message.
type testServer struct {
	port int

	mu       sync.Mutex
	messages []capture
	// rejectData makes the server refuse DATA with a 554.
	rejectData bool
	// dataReply, when set, is the full reply line the server gives to DATA.
	dataReply string
}

func newTestServer(ctx context.Context, t *testing.T, rejectData bool) *testServer {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ts := &testServer{port: ln.Addr().(*net.TCPAddr).Port, rejectData: rejectData}

	srv := gosmtp.NewServer(ts)
	srv.Domain = "localhost"
	srv.AllowInsecureAuth = true
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 10 * time.Second

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		// Close the listener too: srv.Close() misses it when Serve has not started yet.
		_ = ln.Close()
		_ = srv.Close()
		<-done
	})
	return ts
}

func (s *testServer) NewSession(*gosmtp.Conn) (gosmtp.Session, error) {
	return &session{srv: s}, nil
}

func (s *testServer) captured() []capture {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]capture(nil), s.messages...)
}

// session is one SMTP conversation; it implements gosmtp.AuthSession.
type session struct {
	srv *testServer
	cur capture
}

func (s *session) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *session) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, username, password string) error {
		s.cur.username, s.cur.password = username, password
		return nil
	}), nil
}

func (s *session) Mail(from string, _ *gosmtp.MailOptions) error {
	s.cur.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.cur.rcpt = to
	return nil
}

func (s *session) Data(r io.Reader) error {
	if reply := s.srv.dataReply; reply != "" {
		return parseReply(reply)
	}
	if s.srv.rejectData {
		return &gosmtp.SMTPError{Code: 554, EnhancedCode: gosmtp.EnhancedCode{5, 0, 0}, Message: "rejected"}
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.cur.data = string(body)
	s.srv.mu.Lock()
	s.srv.messages = append(s.srv.messages, s.cur)
	s.srv.mu.Unlock()
	return nil
}

// parseReply turns a "454 4.7.0 text" reply line into the error go-smtp sends back.
func parseReply(reply string) error {
	var code, class, subject, detail int
	if _, err := fmt.Sscanf(reply, "%d %d.%d.%d", &code, &class, &subject, &detail); err != nil {
		return fmt.Errorf("bad dataReply %q: %w", reply, err)
	}
	parts := strings.SplitN(reply, " ", 3)
	return &gosmtp.SMTPError{Code: code, EnhancedCode: gosmtp.EnhancedCode{class, subject, detail}, Message: parts[2]}

}

// Reset keeps the credentials: AUTH happens once per connection, before each message.
func (s *session) Reset() {
	s.cur = capture{username: s.cur.username, password: s.cur.password}
}

func (s *session) Logout() error { return nil }

func build(t *testing.T, cfg smtp.Config) messaging.EmailSender {
	t.Helper()
	raw := fmt.Sprintf(`{"host":%q,"port":%d,"username":%q,"password":%q,"from":%q,"fromName":%q}`,
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From, cfg.FromName)
	built, err := smtp.Descriptor().Build([]byte(raw), nil)
	require.NoError(t, err)
	sender, ok := built.(messaging.EmailSender)
	require.True(t, ok)
	return sender
}

func TestSendDeliversWithConfiguredDefaults(t *testing.T) {
	srv := newTestServer(t.Context(), t, false)
	sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com", FromName: "Acme"})

	receipt, err := sender.Send(context.Background(), messaging.EmailMessage{
		To: "rcpt@example.com", Subject: "Hello", Text: "plain body",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, receipt.MessageID)

	got := srv.captured()
	require.Len(t, got, 1)
	assert.Contains(t, got[0].from, "noreply@acme.com", "envelope sender falls back to the integration From")
	assert.Contains(t, got[0].rcpt, "rcpt@example.com")
	assert.Contains(t, got[0].data, "From: \"Acme\" <noreply@acme.com>")
	assert.Contains(t, got[0].data, "Subject: Hello")
	assert.Contains(t, got[0].data, "plain body")
	assert.Contains(t, got[0].data, strings.Trim(receipt.MessageID, "<>"))
	assert.Empty(t, got[0].username, "no credentials configured means no AUTH")
}

func TestSendMessageFromOverridesDefault(t *testing.T) {
	srv := newTestServer(t.Context(), t, false)
	sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com", FromName: "Acme"})

	_, err := sender.Send(context.Background(), messaging.EmailMessage{
		From: "news@acme.com", FromName: "News", To: "rcpt@example.com", Subject: "s", HTML: "<p>x</p>",
	})
	require.NoError(t, err)

	got := srv.captured()
	require.Len(t, got, 1)
	assert.Contains(t, got[0].from, "news@acme.com")
	assert.Contains(t, got[0].data, "News")
}

func TestSendAuthenticatesWhenCredentialsSet(t *testing.T) {
	srv := newTestServer(t.Context(), t, false)
	sender := build(t, smtp.Config{
		Host: "127.0.0.1", Port: srv.port, Username: "user", Password: "secret", From: "noreply@acme.com",
	})

	_, err := sender.Send(context.Background(), messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"})
	require.NoError(t, err)

	got := srv.captured()
	require.Len(t, got, 1)
	assert.Equal(t, "user", got[0].username)
	assert.Equal(t, "secret", got[0].password)
}

func TestSendErrors(t *testing.T) {
	ctx := t.Context()
	msg := messaging.EmailMessage{To: "rcpt@example.com", Subject: "s", Text: "x"}

	t.Run("invalid message", func(t *testing.T) {
		srv := newTestServer(ctx, t, false)
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com"})
		_, err := sender.Send(ctx, messaging.EmailMessage{To: "not-an-address", Text: "x"})
		assert.ErrorContains(t, err, "invalid to address")
		assert.Empty(t, srv.captured())
	})

	t.Run("server rejects DATA", func(t *testing.T) {
		srv := newTestServer(ctx, t, true)
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com"})
		_, err := sender.Send(ctx, msg)
		assert.Error(t, err)
		assert.Empty(t, srv.captured())
	})

	t.Run("transient too-fast reply is busy", func(t *testing.T) {
		srv := newTestServer(ctx, t, false)
		srv.dataReply = "454 4.7.0 Throttling failure: Maximum sending rate exceeded."
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com"})
		_, err := sender.Send(ctx, msg)
		assert.ErrorIs(t, err, messaging.ErrBusy)
		assert.NotErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("transient daily quota reply is quota exceeded", func(t *testing.T) {
		srv := newTestServer(ctx, t, false)
		srv.dataReply = "454 4.7.0 Throttling failure: Daily message quota exceeded."
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com"})
		_, err := sender.Send(ctx, msg)
		assert.ErrorIs(t, err, messaging.ErrQuotaExceeded)
	})

	t.Run("permanent reply is never busy", func(t *testing.T) {
		srv := newTestServer(ctx, t, false)
		srv.dataReply = "554 5.7.1 rate limit policy violation, rejected"
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: srv.port, From: "noreply@acme.com"})
		_, err := sender.Send(ctx, msg)
		require.Error(t, err)
		assert.NotErrorIs(t, err, messaging.ErrBusy)
	})

	t.Run("unreachable host", func(t *testing.T) {
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := ln.Addr().(*net.TCPAddr).Port
		require.NoError(t, ln.Close())
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: port, From: "noreply@acme.com"})
		_, err = sender.Send(ctx, msg)
		assert.Error(t, err)
	})

	t.Run("invalid client options", func(t *testing.T) {
		// A port outside 1..65535 is rejected by go-mail when building the client.
		sender := build(t, smtp.Config{Host: "127.0.0.1", Port: 70000, From: "noreply@acme.com"})
		_, err := sender.Send(ctx, msg)
		assert.ErrorContains(t, err, "smtp: client")
	})
}

func TestDefaultFrom(t *testing.T) {
	sender := build(t, smtp.Config{Host: "h", Port: 25, From: "noreply@acme.com", FromName: "Acme"})
	df, ok := sender.(messaging.DefaultFromer)
	require.True(t, ok)
	addr, name := df.DefaultFrom()
	assert.Equal(t, "noreply@acme.com", addr)
	assert.Equal(t, "Acme", name)
}

func TestDescriptorValidate(t *testing.T) {
	d := smtp.Descriptor()
	assert.Equal(t, messaging.ChannelEmail, d.Channel)
	assert.Equal(t, messaging.ProviderSMTP, d.Provider)

	cases := map[string]struct {
		cfg     string
		wantErr string
	}{
		"valid":        {`{"host":"h","port":25,"from":"a@b.com"}`, ""},
		"malformed":    {`{`, "invalid config"},
		"missing host": {`{"port":25,"from":"a@b.com"}`, "host is required"},
		"bad port":     {`{"host":"h","port":0,"from":"a@b.com"}`, "port must be positive"},
		"missing from": {`{"host":"h","port":25}`, "from is required"},
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
	_, err := smtp.Descriptor().Build([]byte(`{`), nil)
	assert.ErrorContains(t, err, "invalid config")
}
