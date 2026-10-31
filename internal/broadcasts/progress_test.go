package broadcasts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
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

// capAcme sets Acme's Send cap (ADR 0067), as a Staff member does.
func capAcme(t *testing.T, env *testhelper.TestEnv, sendCap int) {
	t.Helper()
	_, err := suspension.SetSendCap(context.Background(), env.Bus, fixtures.AcmeID, suspension.StaffMember("op-1"), &sendCap)
	require.NoError(t, err)
}

// Recipients beyond the Send cap's tokens go out at the Send cap per 24 hours: when that finishes
// after the Provider limit's pace, the estimate is the Send cap's and the progress names it.
func TestProgressNamesTheSendCapWhenItFinishesLater(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	f := sendingWithPending(t, env, 3)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(ctx)
	capAcme(t, env, 2)

	now := time.Now()
	detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), now)
	require.NoError(t, err)

	p := detail.Progress
	assert.Equal(t, broadcasts.LimitSendCap, p.LimitedBy)
	require.NotNil(t, p.EstimatedCompletion)
	assert.WithinDuration(t, now.Add(12*time.Hour), *p.EstimatedCompletion, time.Second,
		"two go out on the bucket's tokens, the third when it holds one again: 12 hours at 2 a day")
}

// A Send cap whose tokens cover the remaining recipients slows nothing: the Provider limit's pace
// is the estimate and the progress names it.
func TestProgressNamesTheProviderLimitWhileTheSendCapCoversTheRest(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	f := sendingWithPending(t, env, 3)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(ctx)
	capAcme(t, env, 1000)

	now := time.Now()
	detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), now)
	require.NoError(t, err)

	p := detail.Progress
	assert.Equal(t, broadcasts.LimitProviderLimit, p.LimitedBy)
	require.NotNil(t, p.EstimatedCompletion)
	assert.WithinDuration(t, now.Add(1500*time.Millisecond), *p.EstimatedCompletion, time.Millisecond)
}

// A Send cap limits even a route the Provider limit does not pace.
func TestProgressNamesTheSendCapWithoutAProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	f := sendingWithPending(t, env, 3)
	capAcme(t, env, 1)

	now := time.Now()
	detail, err := detailOf(env).Detail(ctx, env.DB.Scoped(fixtures.AcmeID), env.DB.Broadcast.GetX(ctx, f.id), now)
	require.NoError(t, err)

	p := detail.Progress
	assert.Equal(t, broadcasts.LimitSendCap, p.LimitedBy)
	require.NotNil(t, p.EstimatedCompletion)
	assert.WithinDuration(t, now.Add(48*time.Hour), *p.EstimatedCompletion, time.Second, "one now, two more a day apart")
}
