package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation an active Rule enrolls Contacts in is not archived: the refusal names the Rule
// (ADR 0099, ADR 0100). A Rule made inactive no longer holds it: its Publish refuses an archived
// Automation.
func TestAnAutomationAnActiveRuleEnrollsInIsNotArchived(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	named := []liveuse.User{{Kind: liveuse.Rule, ID: fixtures.RuleEnrollWaitFirstID, Name: fixtures.RuleEnrollWaitFirstName}}

	_, err := m.Archive(ctx, s, fixtures.AutomationJobsWaitFirstID)
	refused, ok := liveuse.As(err)
	require.True(t, ok)
	assert.Equal(t, named, refused.Users)
	kept, err := m.Get(ctx, s, fixtures.AutomationJobsWaitFirstID)
	require.NoError(t, err)
	assert.Nil(t, kept.ArchivedAt)

	_, err = rules.New(env.Cipher, env.Bus).Deactivate(ctx, s, fixtures.RuleEnrollWaitFirstID)
	require.NoError(t, err)
	archived, err := m.Archive(ctx, s, fixtures.AutomationJobsWaitFirstID)
	require.NoError(t, err, "an inactive Rule holds nothing")
	assert.NotNil(t, archived.ArchivedAt)
}
