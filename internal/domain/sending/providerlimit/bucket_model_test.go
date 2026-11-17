package providerlimit_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
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

// drawLimits draws the ceilings of a run: per second, per day or both.
func drawLimits(t *rapid.T) providerlimit.Limits {
	var limits providerlimit.Limits
	if rapid.Bool().Draw(t, "limited per second") {
		limits.PerSecond = new(rapid.IntRange(1, 20).Draw(t, "per second"))
	}
	if limits.PerSecond == nil || rapid.Bool().Draw(t, "limited per day") {
		limits.PerDay = new(rapid.IntRange(1, 5000).Draw(t, "per day"))
	}
	return limits
}

// limiterRun drives the limiter and the model side by side: each exported method is an action of
// the state machine rapid draws.
type limiterRun struct {
	s           *ent.Scoped
	within      providerlimit.Tx
	reservation providerlimit.Reservation
	model       *bucketModel
	now         time.Time
}

// buckets spends at the run's clock as it stands at each call.
func (r *limiterRun) buckets() *providerlimit.Buckets {
	return providerlimit.NewBuckets(r.within, func() time.Time { return r.now })
}

// Reserve asks for a token: granted exactly when the model holds one in each bucket, and when
// denied, granted after the wait the limiter reports.
func (r *limiterRun) Reserve(t *rapid.T) {
	granted, borderline := r.model.shortBy(r.now)

	short, err := r.buckets().Reserve(t.Context(), r.s, r.reservation)

	require.NoErrorf(t, err, "reserve at %v", r.now)
	assert.Truef(t, borderline || (short.Wait == 0) == granted,
		"at %v the model grants %v, the limiter waited %v", r.now, granted, short.Wait)
	if short.Wait != 0 {
		r.now = r.now.Add(short.Wait)
		again, err := r.buckets().Reserve(t.Context(), r.s, r.reservation)
		require.NoError(t, err)
		require.Zerof(t, again.Wait, "after the reported wait of %v the reservation still waits", short.Wait)
	}
	r.model.take(r.now)
}

// Spend takes a token for a transactional send, granted or not.
func (r *limiterRun) Spend(t *rapid.T) {
	require.NoErrorf(t, r.buckets().Spend(t.Context(), r.s, r.reservation), "spend at %v", r.now)
	r.model.take(r.now)
}

// LetTimePass moves the clock by up to two minutes.
func (r *limiterRun) LetTimePass(t *rapid.T) {
	r.now = r.now.Add(time.Duration(rapid.IntRange(0, 120_000).Draw(t, "milliseconds")) * time.Millisecond)
}

// Check holds nothing between actions: each action checks what it did.
func (*limiterRun) Check(*rapid.T) {}

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
		limits := drawLimits(t)
		run := &limiterRun{
			s:           s,
			within:      env.Bus.WithinAuditTx,
			reservation: providerlimit.Reservation{IntegrationID: fixtures.IntegrationAcmeDefaultID, Limits: limits},
			model:       &bucketModel{limits: limits, second: 1, day: 1, at: start},
			now:         start,
		}

		t.Repeat(rapid.StateMachineActions(run))
	})
}
