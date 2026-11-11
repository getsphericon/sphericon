package rules_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Two Saves of a Rule made on one revision: the second is refused with ErrRevisionChanged, and the
// draft's Condition is the first's (ADR 0119).
func TestASaveOfARuleOnARevisionAlreadyMovedPastIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus, nil, time.Now)
	ctx := context.Background()
	read := env.RuleRevision(ctx, t, fixtures.RuleVipOnlyID)
	first, second := conditionOf(2), conditionOf(3)

	saved, err := m.Update(ctx, acme(env), fixtures.RuleVipOnlyID, rules.UpdateInput{Revision: read, Condition: &first})
	require.NoError(t, err)
	_, err = m.Update(ctx, acme(env), fixtures.RuleVipOnlyID, rules.UpdateInput{Revision: read, Condition: &second})

	require.ErrorIs(t, err, rules.ErrRevisionChanged)
	stored, err := m.Get(ctx, acme(env), fixtures.RuleVipOnlyID)
	require.NoError(t, err)
	assert.JSONEq(t, first, *draftOf(t, stored).Condition)
	assert.Equal(t, saved.Revision, stored.Revision)
}
