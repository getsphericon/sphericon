package testhelper

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/mail"
	"sync"
	"testing"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/cleanup"
)

// MailSink is an SMTP relay on a loopback port that accepts every message and keeps it, standing
// in for the relay the system mail goes through (mailpit in development). It offers no STARTTLS
// and no AUTH, like a local relay. Point SPHERICON_SMTP_HOST and SPHERICON_SMTP_PORT at it.
type MailSink struct {
	// Host and Port are where the relay listens.
	Host string
	Port int

	mu       sync.Mutex
	messages []SunkMail
}

// SunkMail is one message the sink accepted: its envelope recipients and its headers.
type SunkMail struct {
	Rcpt   []string
	Header mail.Header
}

// NewMailSink starts a sink that stops when the test ends.
func NewMailSink(t *testing.T) *MailSink {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok, "a TCP listener has a TCP address")
	sink := &MailSink{Host: addr.IP.String(), Port: addr.Port}

	srv := gosmtp.NewServer(sink)
	srv.Domain = "localhost"
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 10 * time.Second
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Closing the server and its listener is how the test stops it.
		if err := srv.Serve(ln); err != nil && !errors.Is(err, gosmtp.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			cleanup.Report("serve the mail sink", err)
		}
	}()
	t.Cleanup(func() {
		// Close the listener too: srv.Close() misses it when Serve has not started yet.
		cleanup.Close(ln, "the mail sink listener")
		cleanup.Close(srv, "the mail sink")
		<-done
	})
	return sink
}

// Messages returns what the sink has accepted so far, in order.
func (s *MailSink) Messages() []SunkMail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SunkMail(nil), s.messages...)
}

// NewSession implements gosmtp.Backend.
func (s *MailSink) NewSession(*gosmtp.Conn) (gosmtp.Session, error) {
	return &sinkSession{sink: s}, nil
}

// sinkSession is one SMTP conversation with the sink.
type sinkSession struct {
	sink *MailSink
	rcpt []string
}

func (s *sinkSession) Mail(string, *gosmtp.MailOptions) error { return nil }

func (s *sinkSession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.rcpt = append(s.rcpt, to)
	return nil
}

func (s *sinkSession) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	s.sink.mu.Lock()
	s.sink.messages = append(s.sink.messages, SunkMail{Rcpt: s.rcpt, Header: msg.Header})
	s.sink.mu.Unlock()
	return nil
}

func (s *sinkSession) Reset() { s.rcpt = nil }

func (s *sinkSession) Logout() error { return nil }
