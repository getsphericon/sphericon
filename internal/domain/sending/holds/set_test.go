package holds_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

func (w world) input() holds.Input {
	return holds.Input{ReasonCode: "review", Message: "held", ExpiresAt: w.now.Add(time.Hour)}
}

func (w world) limits() holds.Limits {
	return holds.Limits{Now: w.now, MaxExpiry: holds.DefaultMaxExpiry}
}

// A hold is checked by its owner whoever sets it: the Staff console's service endpoint, or any
// other caller that skips the contract (ADR 0107).
func TestSettingAHoldRefusesWhatItsRulesRefuse(t *testing.T) {
	w := newWorld(t)
	for name, tc := range map[string]struct {
		edit func(*holds.Input)
		want error
	}{
		"a reason code with capitals": {func(in *holds.Input) { in.ReasonCode = "Review" }, validation.ErrPatternMismatch},
		"an empty reason code":        {func(in *holds.Input) { in.ReasonCode = "" }, validation.ErrTooShort},
		"a reason code too long":      {func(in *holds.Input) { in.ReasonCode = strings.Repeat("a", 65) }, validation.ErrTooLong},
		"an empty message":            {func(in *holds.Input) { in.Message = "" }, validation.ErrTooShort},
		"a message too long":          {func(in *holds.Input) { in.Message = strings.Repeat("m", 1001) }, validation.ErrTooLong},
		"an http action URL":          {func(in *holds.Input) { in.ActionURL = new("http://example.com/fix") }, validation.ErrPatternMismatch},
		"an action URL with a user":   {func(in *holds.Input) { in.ActionURL = new("https://u:p@example.com/fix") }, holds.ErrActionURL},
		"an expiry in the past":       {func(in *holds.Input) { in.ExpiresAt = w.now.Add(-time.Minute) }, holds.ErrExpiryInPast},
		"an expiry too far ahead":     {func(in *holds.Input) { in.ExpiresAt = w.now.Add(holds.DefaultMaxExpiry + time.Hour) }, holds.ErrTooLong},
	} {
		t.Run(name, func(t *testing.T) {
			in := w.input()
			tc.edit(&in)

			_, err := holds.Set(t.Context(), w.s, "review", "service-a", in, w.limits())

			require.ErrorIs(t, err, tc.want)
		})
	}
	assert.Empty(t, keys(w.standing(t, true).Staff), "a refused hold is not set")
}

func TestSettingAHoldTwiceChangesNothingTheSecondTime(t *testing.T) {
	w := newWorld(t)

	first, err := holds.Set(w.t.Context(), w.s, "review", "service-a", w.input(), w.limits())
	require.NoError(t, err)
	second, err := holds.Set(w.t.Context(), w.s, "review", "service-a", w.input(), w.limits())
	require.NoError(t, err)

	assert.True(t, first.Changed)
	assert.False(t, second.Changed)
	assert.Equal(t, first.Hold.ID, second.Hold.ID)
}

func TestSettingAHoldWithNewTermsRenewsTheSameRow(t *testing.T) {
	w := newWorld(t)
	first, err := holds.Set(w.t.Context(), w.s, "review", "service-a", w.input(), w.limits())
	require.NoError(t, err)
	renewed := w.input()
	renewed.Message = "still held"

	second, err := holds.Set(w.t.Context(), w.s, "review", "service-b", renewed, w.limits())
	require.NoError(t, err)

	assert.True(t, second.Changed)
	assert.Equal(t, first.Hold.ID, second.Hold.ID)
	assert.Equal(t, "still held", second.Hold.Message)
	assert.Equal(t, "service-b", second.Hold.SetBy)
}

func TestClearingAHoldRemovesItAndClearingAMissingOneSucceeds(t *testing.T) {
	w := newWorld(t)
	_, err := holds.Set(w.t.Context(), w.s, "review", "service-a", w.input(), w.limits())
	require.NoError(t, err)

	cleared, err := holds.Clear(w.t.Context(), w.s, "review")
	require.NoError(t, err)
	again, err := holds.Clear(w.t.Context(), w.s, "review")
	require.NoError(t, err)

	assert.True(t, cleared.Changed)
	assert.False(t, again.Changed)
	assert.Nil(t, again.Hold)
	assert.Empty(t, keys(w.standing(t, true).Staff))
}
