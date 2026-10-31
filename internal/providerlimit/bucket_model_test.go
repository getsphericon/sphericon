package providerlimit_test

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// bucketModel is the specification of the two token buckets in one place: the fill is a fraction
// of the capacity, a second's bucket refills in a second and a day's over a day, a marketing
// send needs one token from each limited bucket, and a transactional send takes one even when
// that leaves a bucket negative.
type bucketModel struct {
	limits providerlimit.Limits
	second float64
	day    float64
	at     time.Time
}

func (m *bucketModel) refill(now time.Time) (second, day float64) {
	elapsed := math.Max(0, now.Sub(m.at).Seconds())
	return math.Min(1, m.second+elapsed), math.Min(1, m.day+elapsed/86400)
}

func (m *bucketModel) take(now time.Time) {
	m.second, m.day = m.refill(now)
	if m.limits.PerSecond != nil {
		m.second -= 1 / float64(*m.limits.PerSecond)
	}
	if m.limits.PerDay != nil {
		m.day -= 1 / float64(*m.limits.PerDay)
	}
	if now.After(m.at) {
		m.at = now
	}
}

// shortBy is how far each limited bucket is from holding a token, and whether the model is
// close enough to a bucket's threshold that rounding could decide either way.
func (m *bucketModel) shortBy(now time.Time) (granted, borderline bool) {
	second, day := m.refill(now)
	granted = true
	check := func(fill float64, capacity *int) {
		if capacity == nil {
			return
		}
		threshold := 1 / float64(*capacity)
		if math.Abs(fill-threshold) < 1e-6 {
			borderline = true
		}
		if fill < threshold-1e-9 {
			granted = false
		}
	}
	check(second, m.limits.PerSecond)
	check(day, m.limits.PerDay)
	return granted, borderline
}

// Whatever mix of marketing sends, transactional sends and waiting, a reservation is granted
// exactly when the model says both buckets hold a token, and a denied one reports a wait after
// which the same reservation is granted.
func TestProviderLimiterFollowsTheTokenBucketModel(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	rapid.Check(t, func(t *rapid.T) {
		// Each run starts from no limiter row, so the first reservation creates it full.
		env.DB.ProviderLimiter.Delete().Where(providerlimiter.IntegrationID(fixtures.IntegrationAcmeDefaultID)).ExecX(t.Context())

		var limits providerlimit.Limits
		if rapid.Bool().Draw(t, "limited per second") {
			limits.PerSecond = new(rapid.IntRange(1, 20).Draw(t, "per second"))
		}
		if limits.PerSecond == nil || rapid.Bool().Draw(t, "limited per day") {
			limits.PerDay = new(rapid.IntRange(1, 5000).Draw(t, "per day"))
		}
		model := &bucketModel{limits: limits, second: 1, day: 1, at: start}
		reservation := providerlimit.Reservation{IntegrationID: fixtures.IntegrationAcmeDefaultID, Limits: limits}
		now := start

		t.Repeat(map[string]func(*rapid.T){
			"reserve": func(t *rapid.T) {
				granted, borderline := model.shortBy(now)

				short, err := providerlimit.Reserve(t.Context(), s, env.Bus.WithinAuditTx, reservation, now)
				wait := short.Wait

				if err != nil {
					t.Fatalf("reserve at %v: %v", now, err)
				}
				if !borderline && (wait == 0) != granted {
					t.Fatalf("at %v the model grants %v, the limiter waited %v", now, granted, wait)
				}
				if wait == 0 {
					model.take(now)
					return
				}
				now = now.Add(wait)
				again, err := providerlimit.Reserve(t.Context(), s, env.Bus.WithinAuditTx, reservation, now)
				if err != nil || again.Wait != 0 {
					t.Fatalf("after the reported wait of %v the reservation still waits %v (error %v)", wait, again, err)
				}
				model.take(now)
			},
			"spend": func(t *rapid.T) {
				if err := providerlimit.Spend(t.Context(), s, env.Bus.WithinAuditTx, reservation, now); err != nil {
					t.Fatalf("spend at %v: %v", now, err)
				}
				model.take(now)
			},
			"let time pass": func(t *rapid.T) {
				now = now.Add(time.Duration(rapid.IntRange(0, 120_000).Draw(t, "milliseconds")) * time.Millisecond)
			},
		})
	})
}
