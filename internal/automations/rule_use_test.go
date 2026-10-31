package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation a kept Rule enrolls Contacts in, by its live version or its draft, is not removed:
// the refusal names the Rule (ADR 0049, ADR 0054, spec #767). An archived Rule does not hold it.
func TestAnAutomationARuleEnrollsInIsNotRemoved(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()
	named := []inuse.User{{Kind: inuse.Rule, ID: fixtures.RuleEnrollWaitFirstID, Name: fixtures.RuleEnrollWaitFirstName}}

	for name, remove := range map[string]func() error{
		"delete":  func() error { return m.Delete(ctx, s, fixtures.AutomationJobsWaitFirstID) },
		"archive": func() error { _, err := m.Archive(ctx, s, fixtures.AutomationJobsWaitFirstID); return err },
	} {
		refused, ok := inuse.As(remove())
		require.True(t, ok, name)
		assert.Equal(t, named, refused.Users, name)
	}
	_, err := m.Get(ctx, s, fixtures.AutomationJobsWaitFirstID)
	require.NoError(t, err)

	_, err = rules.New(env.Cipher, env.Bus).Archive(ctx, s, fixtures.RuleEnrollWaitFirstID)
	require.NoError(t, err)
	require.NoError(t, m.Delete(ctx, s, fixtures.AutomationJobsWaitFirstID), "an archived Rule holds nothing")
}
