package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestNewWiresConfigAndServer(t *testing.T) {
	a := newApp(t)

	require.NotNil(t, a.Config)
	assert.True(t, a.Config.IsDev)
	assert.Equal(t, ":"+a.Config.Port, a.Server.Addr)
	require.NotNil(t, a.Server.Handler)

	// The wired handler is the real server: the SPA API refuses an anonymous caller.
	rec := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/site/workspaces", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestNewFailsOnABadEncryptionKey(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_ENCRYPTION_KEY", "not-a-keyset")
	_, err := app.New("test")
	require.Error(t, err)
}

func TestShutdownIsIdempotentAndReportsOnce(t *testing.T) {
	a := newApp(t)
	first := a.Shutdown(context.Background())
	require.NotNil(t, first)
	assert.Empty(t, first.Errors)
	assert.Same(t, first, a.Shutdown(context.Background()), "a second shutdown returns the first report")
}

func TestCommandAppHasNothingToServe(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_METRICS_ADDR", freeAddr(t))
	a, err := app.NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })

	require.NotNil(t, a.Config)
	assert.Nil(t, a.Server)
	assert.Nil(t, a.Metrics, "staff commands expose no metrics listener")
	assert.ErrorIs(t, a.Start(context.Background()), app.ErrNotServable)
}

func TestE2EDKIMLookupOptionIsRefusedOutsideTheE2EProfile(t *testing.T) {
	for _, env := range []string{"production", "development", "test", ""} {
		t.Run(env, func(t *testing.T) {
			a, err := app.New(env, app.WithE2EDKIMLookup())
			require.ErrorIs(t, err, app.ErrE2EOnly)
			assert.Nil(t, a)
		})
	}
}

// relayTo points the system mail at a relay on host:port.
func relayTo(t *testing.T, host string, port int) {
	t.Helper()
	t.Setenv("SPHERICON_SMTP_HOST", host)
	t.Setenv("SPHERICON_SMTP_PORT", strconv.Itoa(port))
	t.Setenv("SPHERICON_SMTP_USER", "")
	t.Setenv("SPHERICON_SMTP_PASS", "")
}

func kramerica(t *testing.T, client *ent.Client) *ent.Workspace {
	t.Helper()
	return client.Workspace.Query().Where(workspace.PublicID(fixtures.KramericaPublicID)).OnlyX(t.Context())
}

func TestSuspendAndUnsuspendWorkspace(t *testing.T) {
	sink := testhelper.NewMailSink(t)
	relayTo(t, sink.Host, sink.Port)
	a, client := newCommandApp(t)
	ctx := context.Background()

	changed, err := a.SuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.StaffMember("op-42"), "complaint rate")
	require.NoError(t, err)
	assert.True(t, changed)
	ws := kramerica(t, client)
	require.NotNil(t, ws.SuspendedAt)
	assert.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)
	assert.Equal(t, "op-42", *ws.SuspendedByID)
	assert.Equal(t, "complaint rate", *ws.SuspensionReason)
	notices := sink.Messages()
	require.Len(t, notices, 1, "the owner is told")
	assert.Equal(t, []string{fixtures.KramericaOwnerNewmanEmail}, notices[0].Rcpt)

	changed, err = a.SuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI, "again")
	require.NoError(t, err)
	assert.False(t, changed, "already suspended: nothing changes")
	assert.Len(t, sink.Messages(), 1, "and the owner is not told twice")
	assert.Equal(t, "complaint rate", *kramerica(t, client).SuspensionReason)

	changed, err = a.UnsuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Nil(t, kramerica(t, client).SuspendedAt)

	changed, err = a.UnsuspendWorkspace(ctx, fixtures.KramericaPublicID, suspension.CLI)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestSuspendWorkspaceKeepsTheSuspensionWhenTheNoticeFails(t *testing.T) {
	// Nothing listens on this port: the suspension must still stand.
	relayTo(t, "127.0.0.1", freePort(t))
	a, client := newCommandApp(t)

	changed, err := a.SuspendWorkspace(context.Background(), fixtures.KramericaPublicID, suspension.CLI, "abuse")
	assert.True(t, changed)
	require.ErrorContains(t, err, "workspace suspended, but the owner notice could not be sent")

	assert.NotNil(t, kramerica(t, client).SuspendedAt)
}

func TestSuspendAndUnsuspendRejectBadInput(t *testing.T) {
	a, _ := newCommandApp(t)
	ctx := context.Background()

	_, err := a.SuspendWorkspace(ctx, "NoSuchId", suspension.CLI, "why")
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
	a, client := newCommandApp(t)
	ctx := context.Background()
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
	// What each entry records, without the moment and the request it came from.
	recorded := lo.Map(envelopes, func(e events.Envelope, _ int) events.AuditEntry {
		ev, err := events.Decode(e)
		require.NoError(t, err)
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return events.AuditEntry{WorkspaceID: entry.WorkspaceID, Action: entry.Action, Actor: events.Actor{Kind: entry.Actor.Kind}}
	})
	reset := func(workspaceID int64) events.AuditEntry {
		return events.AuditEntry{WorkspaceID: workspaceID, Action: events.ActionUserSecondFactorReset, Actor: events.Actor{Kind: events.ActorStaff}}
	}
	assert.ElementsMatch(t, []events.AuditEntry{reset(fixtures.GlobexID), reset(fixtures.UmbrellaID)}, recorded,
		"a reset by Staff, recorded in every Workspace Sam belongs to")

	changed, err = a.ResetSecondFactor(ctx, fixtures.SecondFactorSamEmail)
	require.NoError(t, err)
	assert.False(t, changed, "no second factor: nothing changes")

	_, err = a.ResetSecondFactor(ctx, "nobody@app-2fa-reset.test")
	assert.ErrorContains(t, err, `user "nobody@app-2fa-reset.test"`)
}
