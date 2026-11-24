package rules_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Rule whose Condition counts opens does not fire for a Contact whose engagement is unknown
// (ADR 0123): RuleMassiveUnengaged fires for Ann, who opened nothing, until her address refuses
// tracking.
func TestARuleWithAnEngagementCountDoesNotFireForAnUntrackedContact(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.MassiveID)
	m := rules.New(env.Cipher, env.Bus, nil, time.Now)
	checkIn := func(sourceID string) []int64 {
		t.Helper()
		fired, err := m.Evaluate(t.Context(), s, rules.Event{
			ContactID: fixtures.MassiveAnnID, Action: "unengaged_check", SourceID: sourceID, RecordedAt: time.Now(),
		})
		require.NoError(t, err)
		return ruleIDsOf(t, env, fired)
	}
	tracked := checkIn("evt-tracked")

	_, err := testhelper.Module[*engagementtracking.Module](env).Record(t.Context(), s, engagementtracking.RecordInput{
		Channel: channels.Email, Destination: fixtures.MassiveAnnEmail, State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
	untracked := checkIn("evt-untracked")

	assert.Equal(t, []int64{fixtures.RuleMassiveUnengagedID}, tracked)
	assert.Empty(t, untracked)
}
