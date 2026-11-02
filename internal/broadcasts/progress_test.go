package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sendingWithPending puts the draft Broadcast into sending with n pending recipients
// and no scheduled job ahead, so the ETA falls back to the Integration's rate.
func sendingWithPending(t *testing.T, env *testhelper.TestEnv, n int) *broadcastsFixture {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	id := int64(fixtures.BroadcastDraftID)
	require.NoError(t, s.Broadcast().UpdateOneID(id).SetStatus(broadcast.StatusSending).Exec(ctx))
	contacts := []int64{fixtures.ContactAliceID, fixtures.ContactBobID, fixtures.ContactCarolID}
	for _, c := range contacts[:n] {
		env.PlaceRecipient(ctx, id, c)
	}
	return &broadcastsFixture{id: id}
}

type broadcastsFixture struct{ id int64 }

// detailOf is the module that reads a Broadcast's figures and progress, over the env's Outbound send.
func detailOf(env *testhelper.TestEnv) *broadcasts.Module {
	return broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule())
}

// With no job scheduled ahead the ETA is the remaining recipients at the effective
// rate, and the effective rate includes the provider-discovered ceiling (ADR 0023).
func TestProgressETAFallsBackToTheEffectiveRate(t *testing.T) {
	cases := []struct {
		name   string
		manual *int
		prov   *int
		want   time.Duration // for 3 pending
	}{
		{"manual only", new(2), nil, 1500 * time.Millisecond},
		{"provider only", nil, new(4), 750 * time.Millisecond},
		{"provider lower than manual", new(10), new(1), 3 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			f := sendingWithPending(t, env, 3)
			upd := env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID)
			upd.SetNillableMaxPerSecond(c.manual).SetNillableProviderMaxPerSecond(c.prov).ExecX(ctx)
			b := env.DB.Broadcast.GetX(ctx, f.id)

			now := time.Now()
			detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), b, now)
			require.NoError(t, err)
			p := detail.Progress
			require.NotNil(t, p.EstimatedCompletion)
			assert.WithinDuration(t, now.Add(c.want), *p.EstimatedCompletion, time.Millisecond)
		})
	}
}

// Without any ceiling there is nothing to pace by, so no ETA.
func TestProgressHasNoETAWithoutALimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	f := sendingWithPending(t, env, 3)
	detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), time.Now())
	require.NoError(t, err)
	p := detail.Progress
	require.NotNil(t, p)
	assert.Equal(t, 3, p.Remaining)
	assert.Nil(t, p.EstimatedCompletion)
	assert.Empty(t, p.LimitedBy, "nothing limits it")
}

// A Broadcast whose Workspace has a Send cap says the Send cap is what slows it when its bucket holds
// fewer tokens than recipients remain, and estimates the rest at the Send cap per 24 hours: the
// first recipients go out at once (the bucket is full), the rest one per 24h/cap. Without a
// Provider limit the Send cap alone gives the estimate.
func TestProgressNamesTheSendCapAndEstimatesTheRecipientsBeyondItsTokens(t *testing.T) {
	cases := []struct {
		name    string
		sendCap int
		want    time.Duration // for 3 pending, a full bucket
	}{
		{"two of three fit today", 2, 12 * time.Hour},
		{"one of three fits today", 1, 48 * time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			f := sendingWithPending(t, env, 3)
			env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(c.sendCap).ExecX(ctx)

			now := time.Now()
			detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), now)
			require.NoError(t, err)
			p := detail.Progress
			assert.Equal(t, providerlimit.CauseSendCap, p.LimitedBy)
			require.NotNil(t, p.EstimatedCompletion)
			assert.WithinDuration(t, now.Add(c.want), *p.EstimatedCompletion, time.Second)
		})
	}
}

// With both a Provider limit and a Send cap, the later of the two estimates is reported and names
// its limit: the planner paces by the Provider limit (a recorded last scheduled time, or the rate),
// and the Send cap only wins once the recipients outgrow its bucket.
func TestProgressReportsTheLaterOfTheProviderLimitAndTheSendCap(t *testing.T) {
	cases := []struct {
		name          string
		sendCap       int
		perSecond     *int
		lastScheduled time.Duration // from now; a passed one leaves the estimate to the rate
		want          time.Duration // for 3 pending, a full bucket
		wantLimit     providerlimit.Cause
	}{
		{"the Send cap outlasts the rate", 2, new(1), -time.Hour, 12 * time.Hour, providerlimit.CauseSendCap},
		{"the bucket holds every recipient", 100, new(1), -time.Hour, 3 * time.Second, providerlimit.CauseProviderLimit},
		{"the Send cap outlasts the schedule", 2, nil, time.Hour, 12 * time.Hour, providerlimit.CauseSendCap},
		{"the schedule outlasts a Send cap with room", 3, nil, time.Hour, time.Hour, providerlimit.CauseProviderLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			now := time.Now()
			f := sendingWithPending(t, env, 3)
			env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(c.sendCap).ExecX(ctx)
			env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetNillableMaxPerSecond(c.perSecond).ExecX(ctx)
			env.DB.Broadcast.UpdateOneID(f.id).SetLastScheduledAt(now.Add(c.lastScheduled)).ExecX(ctx)

			detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), now)
			require.NoError(t, err)
			p := detail.Progress
			assert.Equal(t, c.wantLimit, p.LimitedBy)
			require.NotNil(t, p.EstimatedCompletion)
			assert.WithinDuration(t, now.Add(c.want), *p.EstimatedCompletion, time.Second)
		})
	}
}

// Transactional sends spend the Send cap too (ADR 0067), so a bucket they emptied pushes every
// remaining recipient a full 24h/cap further out.
func TestProgressCountsTheSendCapSpentByTransactionalSends(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	f := sendingWithPending(t, env, 3)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(2).ExecX(ctx)
	for _, key := range []string{"tx:1", "tx:2"} {
		res, err := env.OutboundModule().Send(ctx, s, outbound.Request{
			Kind: outboundmessage.KindTransactional, Key: key, Destination: "a@example.com",
			Content: outbound.EmailContent{Subject: "Receipt", Body: "<p>Hi</p>"},
		})
		require.NoError(t, err)
		require.Equal(t, outbound.Sent, res.Outcome)
	}

	now := time.Now()
	detail, err := detailOf(env).Detail(ctx, s, env.DB.Broadcast.GetX(ctx, f.id), now)
	require.NoError(t, err)
	p := detail.Progress
	assert.Equal(t, providerlimit.CauseSendCap, p.LimitedBy)
	require.NotNil(t, p.EstimatedCompletion)
	assert.WithinDuration(t, now.Add(36*time.Hour), *p.EstimatedCompletion, time.Minute)
}

// A self-hosted Instance has no Send cap: one stored on the Workspace (a restored dump) neither
// slows the estimate nor is named as a limit.
func TestProgressIgnoresAStoredSendCapOnSelfhosted(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
	ctx := context.Background()
	f := sendingWithPending(t, env, 3)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(1).ExecX(ctx)

	detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), time.Now())
	require.NoError(t, err)
	p := detail.Progress
	assert.Empty(t, p.LimitedBy)
	assert.Nil(t, p.EstimatedCompletion)
}

// A Broadcast that is not sending has its figures but no Progress, and a sending one gets
// both from the same count.
func TestDetailHasProgressOnlyWhileSending(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	sent, err := detailOf(env).Detail(ctx, s, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastSentID), time.Now())
	require.NoError(t, err)
	assert.Nil(t, sent.Progress)
	assert.Equal(t, 11, sent.Stats.Sent)

	f := sendingWithPending(t, env, 2)
	sending, err := detailOf(env).Detail(ctx, s, env.DB.Broadcast.GetX(ctx, f.id), time.Now())
	require.NoError(t, err)
	require.NotNil(t, sending.Progress)
	assert.Equal(t, sending.Stats.Pending, sending.Progress.Remaining)
	assert.Equal(t, sending.Stats.Processed(), sending.Progress.Processed)
}
