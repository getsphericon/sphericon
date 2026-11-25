package providerlimit_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/providerlimiter"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// modelBucket is one bucket of the reference model: a rate.Limiter whose limit is the ceiling over
// the period and whose burst is the ceiling. Its ceiling is kept here so a run can change it.
type modelBucket struct {
	name    string
	ceiling *int
	period  time.Duration
	lim     *rate.Limiter
}

func (b *modelBucket) limit() rate.Limit {
	return rate.Limit(float64(*b.ceiling) / b.period.Seconds())
}

// bucketModel is the specification of the buckets: golang.org/x/time/rate. A marketing send needs
// one token in every limited bucket and takes one from each or none, so the model checks TokensAt in
// all of them before it takes (AllowN per bucket would spend from one bucket while another is
// short); a Transactional send takes one from each without waiting (ReserveN), which may leave a
// bucket negative. A bucket not spent from
// yet is full at any ceiling. A changed ceiling keeps the balance: the limiter reads the ceilings
// on each call and applies them to the bucket as of its last write (ADR 0023), so each call of
// the model first sets them at that write (SetLimitAt and SetBurstAt), and lifts the burst again
// after it, so a lowered ceiling caps the balance only when a call reads it.
type bucketModel struct {
	buckets []*modelBucket
	written time.Time // the last take; zero before the first
}

// limited is the buckets with a ceiling.
func (m *bucketModel) limited() []*modelBucket {
	out := make([]*modelBucket, 0, len(m.buckets))
	for _, b := range m.buckets {
		if b.ceiling != nil {
			out = append(out, b)
		}
	}
	return out
}

// call runs fn with the ceilings in force, as of the last write.
func (m *bucketModel) call(fn func()) {
	for _, b := range m.limited() {
		b.lim.SetLimitAt(m.written, b.limit())
		b.lim.SetBurstAt(m.written, *b.ceiling)
	}
	fn()
	for _, b := range m.limited() {
		b.lim.SetBurstAt(m.written, math.MaxInt)
	}
}

// wait is how long until every limited bucket holds a token at now (zero when each does), and
// whether a bucket is close enough to one token that rounding could decide either way.
func (m *bucketModel) wait(now time.Time) (wait time.Duration, borderline bool) {
	m.call(func() { wait, borderline = m.waitIn(now) })
	return wait, borderline
}

func (m *bucketModel) waitIn(now time.Time) (wait time.Duration, borderline bool) {
	for _, b := range m.limited() {
		tokens := b.lim.TokensAt(now)
		if math.Abs(tokens-1) < 1e-6 {
			borderline = true
		}
		if tokens < 1 {
			wait = max(wait, time.Duration((1-tokens)/float64(b.limit())*float64(time.Second)))
		}
	}
	return wait, borderline
}

// take spends one token from every limited bucket at now.
func (m *bucketModel) take(now time.Time) {
	m.call(func() {
		for _, b := range m.limited() {
			b.lim.ReserveN(now, 1)
		}
		m.written = now
	})
}

// setCeiling changes the ceiling of b; the next call applies it.
func (m *bucketModel) setCeiling(b *modelBucket, ceiling int) {
	b.ceiling = &ceiling
	if m.written.IsZero() {
		b.lim = rate.NewLimiter(b.limit(), ceiling) // not spent from yet: full
	}
}

func newModelBucket(name string, ceiling *int, period time.Duration) *modelBucket {
	b := &modelBucket{name: name, ceiling: ceiling, period: period}
	if ceiling != nil {
		b.lim = rate.NewLimiter(b.limit(), *ceiling)
	}
	return b
}

// drawLimits draws the ceilings of a run: per second, per day or both, and a Send cap or none.
func drawLimits(t *rapid.T) (limits providerlimit.Limits, sendCap *int) {
	if rapid.Bool().Draw(t, "limited per second") {
		limits.PerSecond = new(rapid.IntRange(1, 20).Draw(t, "per second"))
	}
	if limits.PerSecond == nil || rapid.Bool().Draw(t, "limited per day") {
		limits.PerDay = new(rapid.IntRange(1, 5000).Draw(t, "per day"))
	}
	if rapid.Bool().Draw(t, "send cap") {
		sendCap = new(rapid.IntRange(1, 5000).Draw(t, "send cap per day"))
	}
	return limits, sendCap
}

// limiterRun drives the limiter and the model side by side: each exported method is an action of
// the state machine rapid draws.
type limiterRun struct {
	s      *ent.Scoped
	within providerlimit.Tx
	model  *bucketModel
	now    time.Time
}

// buckets spends at the run's clock as it stands at each call.
func (r *limiterRun) buckets() *providerlimit.Buckets {
	return providerlimit.NewBuckets(r.within, func() time.Time { return r.now })
}

// reservation is what a send spends from, with the ceilings the model holds now.
func (r *limiterRun) reservation() providerlimit.Reservation {
	return providerlimit.Reservation{
		IntegrationID: fixtures.IntegrationAcmeDefaultID,
		Limits:        providerlimit.Limits{PerSecond: r.model.buckets[0].ceiling, PerDay: r.model.buckets[1].ceiling},
		SendCap:       r.model.buckets[2].ceiling,
	}
}

// Reserve asks for a token: granted exactly when the model holds one in each bucket, and when
// denied, with the model's wait (rounded up to a millisecond), after which it is granted.
func (r *limiterRun) Reserve(t *rapid.T) {
	want, borderline := r.model.wait(r.now)

	short, err := r.buckets().Reserve(t.Context(), r.s, r.reservation())

	require.NoErrorf(t, err, "reserve at %v", r.now)
	if !borderline {
		assert.Equalf(t, want == 0, short.Wait == 0, "at %v the model waits %v, the limiter %v", r.now, want, short.Wait)
		if want != 0 && short.Wait != 0 {
			assert.InDeltaf(t, want, short.Wait, float64(2*time.Millisecond), "at %v the wait", r.now)
		}
	}
	if short.Wait != 0 {
		r.now = r.now.Add(short.Wait)
		again, err := r.buckets().Reserve(t.Context(), r.s, r.reservation())
		require.NoError(t, err)
		require.Zerof(t, again.Wait, "after the reported wait of %v the reservation still waits", short.Wait)
	}
	r.model.take(r.now)
}

// Spend takes a token for a Transactional send, granted or not.
func (r *limiterRun) Spend(t *rapid.T) {
	require.NoErrorf(t, r.buckets().Spend(t.Context(), r.s, r.reservation()), "spend at %v", r.now)
	r.model.take(r.now)
}

// LetTimePass moves the clock by up to two minutes, or now and then by up to a day.
func (r *limiterRun) LetTimePass(t *rapid.T) {
	most := 120_000
	if rapid.IntRange(0, 9).Draw(t, "long wait") == 0 {
		most = 86_400_000
	}
	r.now = r.now.Add(time.Duration(rapid.IntRange(0, most).Draw(t, "milliseconds")) * time.Millisecond)
}

// ChangeCeiling raises or lowers the ceiling of one limited bucket.
func (r *limiterRun) ChangeCeiling(t *rapid.T) {
	limited := r.model.limited()
	b := limited[rapid.IntRange(0, len(limited)-1).Draw(t, "bucket")]
	most := 5000
	if b.period == time.Second {
		most = 20
	}
	r.model.setCeiling(b, rapid.IntRange(1, most).Draw(t, b.name))
}

// Check holds nothing between actions: each action checks what it did.
func (*limiterRun) Check(*rapid.T) {}

// Whatever mix of marketing sends, Transactional sends, waiting and changed ceilings, a reservation
// is granted exactly when x/time/rate grants it in every bucket, the Integration's two and the Send
// cap's, and a denied one reports the wait x/time/rate does, after which it is granted.
func TestTheBucketsFollowXTimeRate(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	rapid.Check(t, func(t *rapid.T) {
		// Each run starts from no limiter rows, so the first reservation creates them full.
		env.DB.ProviderLimiter.Delete().Where(providerlimiter.IntegrationID(fixtures.IntegrationAcmeDefaultID)).ExecX(t.Context())
		_, err := s.SendCapLimiter().Delete().Exec(t.Context())
		require.NoError(t, err)
		limits, sendCap := drawLimits(t)
		run := &limiterRun{
			s:      s,
			within: env.Bus.WithinAuditTx,
			model: &bucketModel{buckets: []*modelBucket{
				newModelBucket("per second", limits.PerSecond, time.Second),
				newModelBucket("per day", limits.PerDay, 24*time.Hour),
				newModelBucket("send cap", sendCap, 24*time.Hour),
			}},
			now: start,
		}

		t.Repeat(rapid.StateMachineActions(run))
	})
}
