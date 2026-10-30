package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// RuleTagClicked applies rule-clicked live and rule-clicked-v2 in its draft. Until the draft is
// published, a Firing applies the live Tag (ADR 0065).
func TestAFiringRunsTheLiveVersionNotTheDraft(t *testing.T) {
	env := testhelper.Setup(t)

	fireAndRun(t, env, fixtures.ContactBobID, "rule_click", "evt-live")

	assert.Equal(t, []string{"rule-clicked"}, tagsOf(t, env, fixtures.ContactBobID))
}

func TestAfterPublishTheNextFiringRunsTheNewVersion(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := rules.New(env.Cipher, env.Bus).Publish(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.RuleTagClickedID)
	require.NoError(t, err)

	f := fireAndRun(t, env, fixtures.ContactBobID, "rule_click", "evt-published")

	assert.Equal(t, []string{"rule-clicked-v2"}, tagsOf(t, env, fixtures.ContactBobID))
	assert.EqualValues(t, fixtures.RuleTagClickedDraftVersionID, f.VersionID)
}

// RuleFiringPending fired on the version that was live then: publishing the draft while its Actions
// wait does not change what it runs.
func TestAFiringInProgressKeepsTheVersionItBeganWith(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := rules.New(env.Cipher, env.Bus)
	_, err := m.Publish(ctx, s, fixtures.RuleTagClickedID)
	require.NoError(t, err)

	require.NoError(t, m.RunActions(ctx, s, fixtures.RuleFiringPendingID, ruleRunners(env)))

	assert.Equal(t, []string{"rule-clicked"}, tagsOf(t, env, fixtures.ContactBobID))
}
