//go:build e2e

// Package e2e is the end-to-end suite (ADR 0024): the whole application runs in the
// test process on a real listener, a Workspace's SMTP Integration points at a Mailpit
// started by the suite, everything is driven through the public HTTP API and the
// result is observed only in the Mailpit inbox. Tests never read the database, the
// outbox or an in-memory sender.
//
// Scenarios are written as domain steps on a [Workspace] (ImportContacts,
// SendBroadcast, Inbox.Wait ...); the HTTP transport stays inside this package, so a
// second transport (MCP) can later replay the same scenarios.
package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/internal/accounts"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/events"
)

// Env is the running application under test plus its Mailpit. One Env serves every
// test of the run; tests isolate themselves with their own Workspace.
type Env struct {
	// BaseURL is the application's public URL (the base of every link in an email).
	BaseURL string
	mailpit *Mailpit
	hooks   *Hooks

	accounts *accounts.Accounts
	bus      *events.Bus
	// sessions issues the cookie a signed-in User of the SPA carries.
	sessions *apiauth.Sessions
}

// SystemSenderFrom is the From address of the Instance's System sender in the suite.
const SystemSenderFrom = "noreply@sphericon.e2e.test"

// Boot starts Mailpit and the full application (event router, job queue, HTTP server)
// on a free loopback port and returns the Env with a stop function. The database named
// by DATABASE_URL must already be migrated (the e2e task does that on a fresh one).
func Boot() (*Env, func(), error) {
	ctx := context.Background()
	mp, stopMailpit, err := StartMailpit(ctx)
	if err != nil {
		return nil, nil, err
	}

	// The third-party receiver is up before the application is built: its address is the
	// Instance's outbound allowlist, which the configuration reads once (ADR 0039).
	hooks, stopHooks, err := StartHooks(ctx)
	if err != nil {
		stopMailpit()
		return nil, nil, err
	}
	stopMailpit = stopBoth(stopMailpit, stopHooks)
	if err := os.Setenv("SPHERICON_OUTBOUND_ALLOW", hooks.Addr()); err != nil {
		stopMailpit()
		return nil, nil, err
	}

	// The Instance's System sender is Mailpit too: with no verified Sending domain a Test send
	// goes from it (ADR 0041), and platform mail lands in the same catcher.
	for k, v := range map[string]string{
		"SPHERICON_SMTP_HOST":         mp.SMTPHost,
		"SPHERICON_SMTP_PORT":         strconv.Itoa(mp.SMTPPort),
		"SPHERICON_SYSTEM_EMAIL_FROM": SystemSenderFrom,
	} {
		if err := os.Setenv(k, v); err != nil {
			stopMailpit()
			return nil, nil, err
		}
	}

	// The listener comes first: the public URL (SPHERICON_APP_URL) is derived from its address.
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		stopMailpit()
		return nil, nil, err
	}
	a, err := app.New("e2e", app.WithListener(ln), app.WithE2EDKIMLookup())
	if err != nil {
		_ = ln.Close()
		stopMailpit()
		return nil, nil, fmt.Errorf("start application: %w", err)
	}
	// The arrange step goes through the product's own Accounts module, taken from the
	// application's container (no second pool).
	acc, err := a.Accounts()
	if err != nil {
		_ = a.Close(ctx)
		stopMailpit()
		return nil, nil, fmt.Errorf("resolve accounts: %w", err)
	}
	bus, err := a.Bus()
	if err != nil {
		_ = a.Close(ctx)
		stopMailpit()
		return nil, nil, fmt.Errorf("resolve bus: %w", err)
	}
	// The same start/stop ordering as the binary; Start unwinds itself on failure.
	if err := a.Start(ctx); err != nil {
		stopMailpit()
		return nil, nil, err
	}

	stop := func() {
		cctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = a.Close(cctx)
		stopMailpit()
	}
	return &Env{
		BaseURL:  a.Config.AppURL,
		mailpit:  mp,
		hooks:    hooks,
		accounts: acc,
		bus:      bus,
		sessions: apiauth.NewSessions(a.Config.JWTSecret, a.Config.SessionTTL, a.Config.SecureCookies(), nil, a.Config.JWTVerifySecrets()...),
	}, stop, nil
}

// stopBoth is one stop function for two.
func stopBoth(first, second func()) func() {
	return func() {
		second()
		first()
	}
}
