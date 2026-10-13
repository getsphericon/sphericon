package reputation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const day = 24 * time.Hour

func byDomain(t *testing.T, rates []reputation.DomainRates) map[string]reputation.DomainRates {
	t.Helper()
	out := map[string]reputation.DomainRates{}
	for _, r := range rates {
		out[r.Domain.Domain] = r
	}
	return out
}

// Fixtures (events.yml): in the trailing 7 days mail.acme.com has 8 sent, 1 hard
// bounce (a soft one is excluded) and 1 complaint; older events fall outside.
func TestRatesOverTheTrailingWindow(t *testing.T) {
	env := testhelper.Setup(t)
	rates, err := reputation.New().Rates(context.Background(), env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(7*day))
	require.NoError(t, err)

	mail := byDomain(t, rates)["mail.acme.com"]
	assert.Equal(t, 1, mail.Complaint.Numerator)
	assert.Equal(t, 7, mail.Complaint.Denominator, "complaints are over sent minus hard bounces")
	require.NotNil(t, mail.Complaint.Rate)
	assert.InDelta(t, 1.0/7.0, *mail.Complaint.Rate, 1e-9)
	assert.Equal(t, 1, mail.Bounce.Numerator)
	assert.Equal(t, 8, mail.Bounce.Denominator)
	require.NotNil(t, mail.Bounce.Rate)
	assert.InDelta(t, 1.0/8.0, *mail.Bounce.Rate, 1e-9)
}

func TestRatesListsEveryDomainOrderedByIDWithUndefinedRatesForNoTraffic(t *testing.T) {
	env := testhelper.Setup(t)
	rates, err := reputation.New().Rates(context.Background(), env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(7*day))
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(rates), 3)
	for i := 1; i < len(rates); i++ {
		assert.Less(t, rates[i-1].Domain.ID, rates[i].Domain.ID)
	}

	news := byDomain(t, rates)["news.acme.com"] // unverified, never sent from
	assert.Zero(t, news.Complaint.Denominator)
	assert.Nil(t, news.Complaint.Rate, "no traffic means an undefined rate, not zero")
	assert.Nil(t, news.Bounce.Rate)
}

func TestRatesWindowSelectsByTheEventsOwnTimestamp(t *testing.T) {
	env := testhelper.Setup(t)
	m := reputation.New()

	narrow, err := m.Rates(context.Background(), env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(7*day))
	require.NoError(t, err)
	wide, err := m.Rates(context.Background(), env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(365*day))
	require.NoError(t, err)

	n, w := byDomain(t, narrow)["mail.acme.com"], byDomain(t, wide)["mail.acme.com"]
	assert.Greater(t, w.Bounce.Denominator, n.Bounce.Denominator, "the 40-day-old sends join a wider window")
}

// An Event with no occurred_at falls back to its ingest time (created_at).
func TestRatesFallBackToIngestTime(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	create := func(action string, props map[string]any) {
		env.DB.Event.Create().SetWorkspaceID(fixtures.AcmeID).SetAction(action).SetProperties(props).ExecX(ctx)
	}
	create(events.NameEmailSent, map[string]any{"sendingDomain": "news.acme.com"})
	create(events.NameEmailSent, map[string]any{"sendingDomain": "news.acme.com"})
	create(events.NameEmailBounced, map[string]any{"sendingDomain": "news.acme.com", "bounceKind": events.BounceKindPermanent})
	create(events.NameEmailBounced, map[string]any{"sendingDomain": "news.acme.com", "bounceKind": events.BounceKindTransient})
	create(events.NameEmailComplained, map[string]any{"sendingDomain": "other.example"})

	rates, err := reputation.New().Rates(ctx, env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(day))
	require.NoError(t, err)
	news := byDomain(t, rates)["news.acme.com"]
	assert.Equal(t, 1, news.Bounce.Numerator, "only the permanent bounce counts")
	assert.Equal(t, 2, news.Bounce.Denominator)
	assert.Equal(t, 0, news.Complaint.Numerator, "a complaint on another domain is not counted")
	assert.Equal(t, 1, news.Complaint.Denominator, "sent minus hard bounces")
	require.NotNil(t, news.Complaint.Rate)
	assert.Zero(t, *news.Complaint.Rate)
}

func TestRatesAreWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	rates, err := reputation.New().Rates(context.Background(), env.DB.Scoped(fixtures.GlobexID), reputation.Trailing(7*day))
	require.NoError(t, err)
	require.Len(t, rates, 1, "only Globex's own domain is listed")
	assert.Equal(t, "mail.globex.test", rates[0].Domain.Domain)
	assert.Zero(t, rates[0].Bounce.Denominator, "Acme's events never leak into another workspace")
	assert.Nil(t, rates[0].Bounce.Rate)
}

func TestRatesReportsQueryErrors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := reputation.New().Rates(ctx, env.DB.Scoped(fixtures.AcmeID), reputation.Trailing(7*day))
	assert.Error(t, err)
}

// A failure while counting a later domain's events aborts the whole read.
func TestRatesReportsCountErrors(t *testing.T) {
	env := testhelper.Setup(t)
	for _, n := range []int{1, 2, 3} {
		queries := 0
		client := env.DB
		client.Event.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
				queries++
				if queries == n {
					return nil, assert.AnError
				}
				return next.Query(ctx, q)
			})
		}))
		_, err := reputation.New().Rates(context.Background(), client.Scoped(fixtures.AcmeID), reputation.Trailing(7*day))
		assert.ErrorIs(t, err, assert.AnError, "failing event count #%d", n)
	}
}
