package holds_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/holds"
)

func (w world) input() holds.Input {
	return holds.Input{ReasonCode: "review", Message: "held", ExpiresAt: w.now.Add(time.Hour)}
}

func TestSettingAHoldTwiceChangesNothingTheSecondTime(t *testing.T) {
	w := newWorld(t)

	first, err := holds.Set(w.ctx, w.s, "review", "service-a", w.input())
	require.NoError(t, err)
	second, err := holds.Set(w.ctx, w.s, "review", "service-a", w.input())
	require.NoError(t, err)

	assert.True(t, first.Changed)
	assert.False(t, second.Changed)
	assert.Equal(t, first.Hold.ID, second.Hold.ID)
}

func TestSettingAHoldWithNewTermsRenewsTheSameRow(t *testing.T) {
	w := newWorld(t)
	first, err := holds.Set(w.ctx, w.s, "review", "service-a", w.input())
	require.NoError(t, err)
	renewed := w.input()
	renewed.Message = "still held"

	second, err := holds.Set(w.ctx, w.s, "review", "service-b", renewed)
	require.NoError(t, err)

	assert.True(t, second.Changed)
	assert.Equal(t, first.Hold.ID, second.Hold.ID)
	assert.Equal(t, "still held", second.Hold.Message)
	assert.Equal(t, "service-b", second.Hold.SetBy)
}

func TestClearingAHoldRemovesItAndClearingAMissingOneSucceeds(t *testing.T) {
	w := newWorld(t)
	_, err := holds.Set(w.ctx, w.s, "review", "service-a", w.input())
	require.NoError(t, err)

	cleared, err := holds.Clear(w.ctx, w.s, "review")
	require.NoError(t, err)
	again, err := holds.Clear(w.ctx, w.s, "review")
	require.NoError(t, err)

	assert.True(t, cleared.Changed)
	assert.False(t, again.Changed)
	assert.Nil(t, again.Hold)
	assert.Empty(t, keys(w.standing(t, true).Staff))
}
