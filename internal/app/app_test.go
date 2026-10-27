package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// smtpSink is a minimal SMTP server that records the RCPT of every accepted message,
// standing in for the platform's system mail relay (mailpit in dev).
type smtpSink struct {
	port int
	mu   sync.Mutex
	rcpt []string
}

func newSMTPSink(t *testing.T) *smtpSink {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &smtpSink{port: ln.Addr().(*net.TCPAddr).Port}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() { cleanup.Close(ln, "ln"); <-done })
	return s
}

func (s *smtpSink) serve(conn net.Conn) {
	defer func() { cleanup.Close(conn, "conn") }()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return
	}
	r := bufio.NewReader(conn)
	say := func(f string, a ...any) { _, _ = fmt.Fprintf(conn, f+"\r\n", a...) }
	say("220 sink")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELLO"):
			say("250 sink")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(line[len("RCPT TO:"):]))
			s.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 go")
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
			}
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (s *smtpSink) recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.rcpt...)
}

// baseline points the app at a database of the test's own, at the fixture state: the
// app connects directly and commits for real, outside the txdb harness every other
// package uses. The database is dropped with the test.
func baseline(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", testhelper.RealDBURL(t))
}

func newApp(t *testing.T) *App {
	t.Helper()
	baseline(t)
	a, err := New("test")
	require.NoError(t, err)

	// Shutting the event router down only completes once it has run, so every app
	// under test runs it for its lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.runEvents(ctx) }()
	select {
	case <-a.events.router.Running():
	case err := <-done:
		cancel()
		t.Fatalf("router stopped before running: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("router never started")
	}
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("event router: %v", err)
		}
		cleanup.Shutdown(a.Shutdown(context.Background()))
	})
	return a
}

func TestNewWiresConfigAndServer(t *testing.T) {
	a := newApp(t)

	require.NotNil(t, a.Config)
	assert.True(t, a.Config.IsDev)
	assert.Equal(t, ":"+a.Config.Port, a.Server.Addr)
	assert.Equal(t, 5*time.Second, a.Server.ReadHeaderTimeout)
	require.NotNil(t, a.Server.Handler)

	// The wired handler is the real server: the SPA API refuses an anonymous caller.
	rec := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/site/workspaces", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestNewFailsOnABadEncryptionKey(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_ENCRYPTION_KEY", "not-a-keyset")
	_, err := New("test")
	require.Error(t, err)
}

func TestShutdownIsIdempotentAndReportsOnce(t *testing.T) {
	a := newApp(t)
	first := a.Shutdown(context.Background())
	require.NotNil(t, first)
	assert.Empty(t, first.Errors)
	assert.Same(t, first, a.Shutdown(context.Background()), "a second shutdown returns the first report")
}

func TestRunEventsStopsWhenTheContextIsCancelled(t *testing.T) {
	baseline(t)
	a, err := New("test")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.runEvents(ctx) }()

	testhelper.Receive(t, a.events.router.Running(), "router never started")
	cancel()
	runErr := testhelper.Receive(t, done, "RunEvents did not return after cancel")
	require.NoError(t, runErr)
	assert.Empty(t, a.Shutdown(context.Background()).Errors)
}

func TestRunJobsStartsTheWorkerPool(t *testing.T) {
	a := newApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, a.runJobs(ctx))
	cancel()
	assert.Empty(t, a.Shutdown(context.Background()).Errors, "workers stop cleanly")
}

func TestNewStaffIsMinimal(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_METRICS_ADDR", freeAddr(t))
	a, err := NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })

	require.NotNil(t, a.Config)
	assert.Nil(t, a.Server)
	assert.Nil(t, a.Metrics, "staff commands expose no metrics listener")
	assert.Nil(t, a.events)
	assert.Nil(t, a.jobs)
}

// freeAddr returns a loopback host:port that was free a moment ago (config rejects
// port 0, so the app cannot be handed an ephemeral one).
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestStopShutsDownTheMetricsServer(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_METRICS_ADDR", freeAddr(t))
	a, err := New("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	require.NotNil(t, a.Metrics)

	require.NoError(t, a.bindMetrics(t.Context()))
	go func() { cleanup.Served("metrics", a.Metrics.Serve()) }()
	url := "http://" + a.Metrics.Addr() + "/metrics"
	require.Eventually(t, func() bool {
		code, _, err := testhelper.TryHTTPGet(t.Context(), url)
		return err == nil && code != 0
	}, 5*time.Second, 20*time.Millisecond, "metrics listener accepts before Stop")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, a.stop(ctx))

	_, _, err = testhelper.TryHTTPGet(t.Context(), url)
	assert.Error(t, err, "metrics listener stopped accepting after Stop")
}

func TestSuspendAndUnsuspendWorkspace(t *testing.T) {
	sink := newSMTPSink(t)
	t.Setenv("SPHERICON_SMTP_HOST", "127.0.0.1")
	t.Setenv("SPHERICON_SMTP_PORT", strconv.Itoa(sink.port))
	t.Setenv("SPHERICON_SMTP_USER", "")
	t.Setenv("SPHERICON_SMTP_PASS", "")
	baseline(t)
	a, err := NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	ctx := context.Background()
	client, err := invokeEnt(a)
	require.NoError(t, err)
	state := func() *ent.Workspace {
		return client.Workspace.Query().Where(workspace.PublicID(fixtures.KramericaPublicID)).OnlyX(ctx)
	}

	changed, err := a.SuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.StaffMember("op-42"), "complaint rate")
	require.NoError(t, err)
	assert.True(t, changed)
	ws := state()
	require.NotNil(t, ws.SuspendedAt)
	assert.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)
	assert.Equal(t, "op-42", *ws.SuspendedByID)
	assert.Equal(t, "complaint rate", *ws.SuspensionReason)
	assert.Len(t, sink.recipients(), 1, "the owner is told")
	assert.Contains(t, sink.recipients()[0], fixtures.KramericaOwnerNewmanEmail)

	changed, err = a.SuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI, "again")
	require.NoError(t, err)
	assert.False(t, changed, "already suspended: nothing changes")
	assert.Len(t, sink.recipients(), 1, "and the owner is not told twice")
	assert.Equal(t, "complaint rate", *state().SuspensionReason)

	changed, err = a.UnsuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Nil(t, state().SuspendedAt)

	changed, err = a.UnsuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestSuspendWorkspaceKeepsTheSuspensionWhenTheNoticeFails(t *testing.T) {
	// Nothing listens on this port: the suspension must still stand.
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	t.Setenv("SPHERICON_SMTP_HOST", "127.0.0.1")
	t.Setenv("SPHERICON_SMTP_PORT", strconv.Itoa(port))
	baseline(t)
	a, err := NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	ctx := context.Background()

	changed, err := a.SuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI, "abuse")
	assert.True(t, changed)
	require.ErrorContains(t, err, "workspace suspended, but the owner notice could not be sent")

	client, cerr := invokeEnt(a)
	require.NoError(t, cerr)
	assert.NotNil(t, client.Workspace.Query().Where(workspace.PublicID(fixtures.KramericaPublicID)).OnlyX(ctx).SuspendedAt)
}

func TestSuspendAndUnsuspendRejectBadInput(t *testing.T) {
	baseline(t)
	a, err := NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	ctx := context.Background()

	_, err = a.SuspendWorkspace(ctx, "NoSuchId", suspension.CLI, "why")
	require.ErrorContains(t, err, `workspace "NoSuchId"`)
	_, err = a.UnsuspendWorkspace(ctx, "NoSuchId", suspension.CLI)
	require.ErrorContains(t, err, `workspace "NoSuchId"`)

	changed, err := a.SuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.Actor{}, "reason")
	assert.False(t, changed)
	assert.ErrorContains(t, err, "an actor is required")
}

// The staff member command resets a fixture User's Second factor for real (this app
// commits outside the per-test transaction), on a database of the test's own.
func TestResetSecondFactorByEmail(t *testing.T) {
	baseline(t)
	a, err := NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	ctx := context.Background()
	client, err := invokeEnt(a)
	require.NoError(t, err)

	before := client.User.GetX(ctx, fixtures.SecondFactorSamID)

	changed, err := a.ResetSecondFactor(ctx, fixtures.SecondFactorSamEmail)
	require.NoError(t, err)
	assert.True(t, changed)
	after := client.User.GetX(ctx, fixtures.SecondFactorSamID)
	assert.Nil(t, after.SecondFactorConfirmedAt)
	assert.Empty(t, after.SecondFactorSecretEncrypted)
	assert.Greater(t, after.SessionEpoch, before.SessionEpoch, "every session ends")

	envelopes, err := testhelper.OutboxEnvelopesOn(ctx, client, events.NameAuditEntry)
	require.NoError(t, err)
	workspaces := make([]int64, 0, len(envelopes))
	for _, e := range envelopes {
		ev, err := events.Decode(e)
		require.NoError(t, err)
		entry := ev.(*events.AuditEntry)
		require.Equal(t, events.ActionUserSecondFactorReset, entry.Action)
		assert.Equal(t, events.ActorStaff, entry.Actor.Kind)
		workspaces = append(workspaces, entry.WorkspaceID)
	}
	assert.ElementsMatch(t, []int64{fixtures.GlobexID, fixtures.UmbrellaID}, workspaces,
		"an Audit entry in every Workspace Sam belongs to")

	changed, err = a.ResetSecondFactor(ctx, fixtures.SecondFactorSamEmail)
	require.NoError(t, err)
	assert.False(t, changed, "no second factor: nothing changes")

	_, err = a.ResetSecondFactor(ctx, "nobody@app-2fa-reset.test")
	assert.ErrorContains(t, err, `user "nobody@app-2fa-reset.test"`)
}

func TestBuildSystemSender(t *testing.T) {
	catalog := registry.Default()
	base := &config.Config{
		SMTPHost: "mail.example.test", SMTPPort: 2525, SMTPFrom: "legacy@example.test",
		SystemEmailFrom: "noreply@example.test",
		SESRegion:       "eu-west-1", SESAccessKeyID: "AKIA", SESSecretAccessKey: "shh",
	}

	t.Run("smtp is the default provider", func(t *testing.T) {
		sender, err := buildSystemSender(base, catalog)
		require.NoError(t, err)
		addr, name := sender.(messaging.DefaultFromer).DefaultFrom()
		assert.Equal(t, "noreply@example.test", addr)
		assert.Equal(t, "sphericon", name)
	})

	t.Run("ses provider", func(t *testing.T) {
		cfg := *base
		cfg.SystemEmailProvider = "ses"
		sender, err := buildSystemSender(&cfg, catalog)
		require.NoError(t, err)
		addr, _ := sender.(messaging.DefaultFromer).DefaultFrom()
		assert.Equal(t, "noreply@example.test", addr)
	})

	t.Run("falls back to SPHERICON_SMTP_FROM when no system From is set", func(t *testing.T) {
		cfg := *base
		cfg.SystemEmailFrom = ""
		sender, err := buildSystemSender(&cfg, catalog)
		require.NoError(t, err)
		addr, _ := sender.(messaging.DefaultFromer).DefaultFrom()
		assert.Equal(t, "legacy@example.test", addr)
	})

	t.Run("provider missing from the catalog fails", func(t *testing.T) {
		_, err := buildSystemSender(base, messaging.NewCatalog())
		assert.Error(t, err)
	})

	t.Run("ses misconfiguration is reported by the catalog", func(t *testing.T) {
		cfg := *base
		cfg.SystemEmailProvider = "ses"
		_, err := buildSystemSender(&cfg, messaging.NewCatalog())
		assert.Error(t, err)
	})
}

func TestBuildSystemSenderPassesTheReplyToToTheProvider(t *testing.T) {
	for _, provider := range []messaging.Provider{messaging.ProviderSMTP, messaging.ProviderSES} {
		t.Run(string(provider), func(t *testing.T) {
			var blob []byte
			catalog := messaging.NewCatalog(messaging.ProviderDescriptor{
				Channel:  messaging.ChannelEmail,
				Provider: provider,
				Build: func(raw []byte, _ messaging.Deps) (any, error) {
					blob = raw
					return struct{ messaging.EmailSender }{}, nil
				},
			})
			cfg := &config.Config{SystemEmailProvider: string(provider), SystemEmailFrom: "noreply@example.test", SystemEmailReplyTo: "replies@example.test"}
			_, err := buildSystemSender(cfg, catalog)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(blob, &got))
			assert.Equal(t, "replies@example.test", got["replyTo"])
		})
	}
}

func TestBuildSystemSenderSendsTheConfiguredFromName(t *testing.T) {
	cfg := &config.Config{SystemEmailFrom: "noreply@example.test", SystemEmailFromName: "Example Mail", SMTPHost: "h", SMTPPort: 25}
	sender, err := buildSystemSender(cfg, registry.Default())
	require.NoError(t, err)
	addr, name := sender.(messaging.DefaultFromer).DefaultFrom()
	assert.Equal(t, "noreply@example.test", addr)
	assert.Equal(t, "Example Mail", name)
}

func TestSystemSenderRejectsAnUnknownProviderViaTheCatalogOnly(t *testing.T) {
	// Any provider name other than "ses" is the SMTP dev path (documented default).
	sender, err := buildSystemSender(&config.Config{SystemEmailProvider: "anything", SMTPHost: "h", SMTPPort: 25, SMTPFrom: "a@b.test"}, registry.Default())
	require.NoError(t, err)
	assert.NotNil(t, sender)
}

func TestAppServesOnACallerProvidedListener(t *testing.T) {
	baseline(t)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	a, err := New("e2e", WithListener(ln))
	require.NoError(t, err)

	base := "http://" + ln.Addr().String()
	assert.Equal(t, base, a.Config.AppURL, "the public URL is derived from the listener")

	// A link the tracker builds points at the listener...
	tracker, err := do.Invoke[*tracking.Tracker](a.injector)
	require.NoError(t, err)
	link := tracker.OpenURL("tok")
	require.True(t, strings.HasPrefix(link, base+"/e/o/"), link)

	ctx, cancel := context.WithCancel(context.Background())
	eventsDone := make(chan error, 1)
	served := make(chan error, 1)
	go func() { eventsDone <- a.runEvents(ctx) }()
	testhelper.Receive(t, a.events.router.Running(), "router never started")
	require.NoError(t, a.bindMetrics(t.Context()))
	go func() { served <- a.serve() }()
	t.Cleanup(func() {
		cancel()
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer scancel()
		cleanup.Report("stop the app", a.stop(sctx))
		cleanup.Shutdown(a.Shutdown(sctx))
	})

	// ...and the running server answers there, on every API surface.
	for _, path := range []string{"/healthz", "/api/auth/me", "/site/workspaces", "/collect/events"} {
		require.Eventually(t, func() bool {
			code, _, err := testhelper.TryHTTPGet(t.Context(), base+path)
			return err == nil && code != http.StatusNotFound
		}, 5*time.Second, 20*time.Millisecond, path)
	}
	code, _, err := testhelper.TryHTTPGet(t.Context(), link)
	require.NoError(t, err)
	assert.NotEqual(t, http.StatusNotFound, code, "a tracker link round-trips to the running server")
}

func TestE2EDKIMLookupOptionIsRefusedOutsideTheE2EProfile(t *testing.T) {
	for _, env := range []string{"production", "development", "test", ""} {
		a, err := New(env, WithE2EDKIMLookup())
		require.ErrorIs(t, err, ErrE2EOnly, env)
		assert.Nil(t, a)
	}
}
