package holds_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func keys(rows []*ent.Hold) []string {
	return lo.Map(rows, func(h *ent.Hold, _ int) string { return h.Key })
}

// The SQL read (Active) and the Go filter over loaded rows (Live) state one rule: a hold
// is active while its expiry is after now, so one expiring exactly now is already lapsed.
func TestLiveAgreesWithActive(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	now := time.Now().Truncate(time.Microsecond) // the precision the database stores

	for key, expires := range map[string]time.Time{
		"future":   now.Add(time.Hour),
		"boundary": now,
		"past":     now.Add(-time.Hour),
	} {
		_, err := s.Hold().Create().SetKey(key).SetReasonCode("review").SetMessage("held").
			SetSetBy("service-a").SetExpiresAt(expires).Save(ctx)
		require.NoError(t, err)
	}

	active, err := holds.Active(ctx, s, now)
	require.NoError(t, err)
	all, err := s.Hold().Query().All(ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"future"}, keys(active))
	assert.Equal(t, keys(active), keys(holds.Live(all, now)))
}
