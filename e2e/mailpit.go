//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Address is a mailbox as Mailpit reports it.
type Address struct {
	Name    string
	Address string
}

// summary is one inbox row, as listed.
type summary struct {
	ID      string
	From    Address
	To      []Address
	Subject string
}

// Mailpit is a thin client over Mailpit's HTTP API (it publishes only Swagger 2.0,
// which the project's OpenAPI generator rejects, and a handful of endpoints are enough).
type Mailpit struct {
	// SMTPHost and SMTPPort are where a Workspace's SMTP Integration should point.
	SMTPHost string
	SMTPPort int

	base string
	hc   *http.Client
	poll time.Duration
}

// URL is the base URL of the Mailpit web UI and HTTP API.
func (m *Mailpit) URL() string { return m.base }

// NewMailpit builds a client for the Mailpit HTTP API at base ("http://127.0.0.1:8025").
func NewMailpit(base string) *Mailpit {
	return &Mailpit{base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: 10 * time.Second}, poll: 100 * time.Millisecond}
}

type apiAddress struct{ Name, Address string }

func (a apiAddress) toAddress() Address { return Address(a) }

func addresses(in []apiAddress) []Address {
	out := make([]Address, len(in))
	for i, a := range in {
		out[i] = a.toAddress()
	}
	return out
}

func (m *Mailpit) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mailpit %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// list returns the newest inbox rows (Mailpit pages at 50 by default; limit raises it).
func (m *Mailpit) list(ctx context.Context, limit int) ([]summary, error) {
	return m.summaries(ctx, "/api/v1/messages", url.Values{"limit": {fmt.Sprint(limit)}})
}

// search returns the inbox rows matching a Mailpit search query (e.g. `to:a@b.test`).
func (m *Mailpit) search(ctx context.Context, query string) ([]summary, error) {
	return m.summaries(ctx, "/api/v1/search", url.Values{"query": {query}, "limit": {"100"}})
}

func (m *Mailpit) summaries(ctx context.Context, path string, q url.Values) ([]summary, error) {
	var resp struct {
		Messages []struct {
			ID      string `json:"ID"`
			From    apiAddress
			To      []apiAddress
			Subject string
		} `json:"messages"`
	}
	if err := m.do(ctx, http.MethodGet, path+"?"+q.Encode(), nil, &resp); err != nil {
		return nil, err
	}
	out := make([]summary, len(resp.Messages))
	for i, s := range resp.Messages {
		out[i] = summary{ID: s.ID, From: s.From.toAddress(), To: addresses(s.To), Subject: s.Subject}
	}
	return out, nil
}

// get loads one message with its body and raw headers.
func (m *Mailpit) get(ctx context.Context, id string) (Message, error) {
	var msg struct {
		ID      string `json:"ID"`
		From    apiAddress
		To      []apiAddress
		Subject string
		Text    string
		HTML    string
	}
	if err := m.do(ctx, http.MethodGet, "/api/v1/message/"+url.PathEscape(id), nil, &msg); err != nil {
		return Message{}, err
	}
	var raw map[string][]string
	if err := m.do(ctx, http.MethodGet, "/api/v1/message/"+url.PathEscape(id)+"/headers", nil, &raw); err != nil {
		return Message{}, err
	}
	headers := make(map[string][]string, len(raw))
	for k, v := range raw {
		headers[http.CanonicalHeaderKey(k)] = v
	}
	return Message{ID: msg.ID, From: msg.From.toAddress(), To: addresses(msg.To), Subject: msg.Subject, Text: msg.Text, HTML: msg.HTML, Headers: headers}, nil
}

// remove deletes the given messages. Parallel tests delete only their own ids, never
// the whole inbox.
func (m *Mailpit) remove(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return m.do(ctx, http.MethodDelete, "/api/v1/messages", map[string][]string{"IDs": ids}, nil)
}

// StartMailpit runs the toolchain's mailpit binary on two free loopback ports and
// returns a client plus a stop function. The inbox keeps every message (--max).
func StartMailpit(ctx context.Context) (*Mailpit, func(), error) {
	smtpPort, err := freePort(ctx)
	if err != nil {
		return nil, nil, err
	}
	httpPort, err := freePort(ctx)
	if err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(ctx, "mailpit",
		"--smtp", fmt.Sprintf("127.0.0.1:%d", smtpPort),
		"--listen", fmt.Sprintf("127.0.0.1:%d", httpPort),
		"--max", "1000000", "--quiet", "--disable-version-check")
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start mailpit (is it on PATH? run through mise): %w", err)
	}
	stop := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	mp := NewMailpit(fmt.Sprintf("http://127.0.0.1:%d", httpPort))
	mp.SMTPHost, mp.SMTPPort = "127.0.0.1", smtpPort

	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		if _, err := mp.list(deadline, 1); err == nil {
			return mp, stop, nil
		}
		select {
		case <-deadline.Done():
			stop()
			return nil, nil, errors.New("mailpit did not become ready")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// freePort asks the kernel for a free loopback port. The tiny window between closing
// the probe and mailpit binding is accepted; mailpit then fails loudly on a clash.
func freePort(ctx context.Context) (int, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("a tcp listener has a %T address", ln.Addr())
	}
	return addr.Port, nil
}
