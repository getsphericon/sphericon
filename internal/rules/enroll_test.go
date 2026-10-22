package rules_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAnEnrollActionNeedsAnAutomationOfTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Bus)
	s := env.DB.Scoped(fixtures.AcmeID)
	create := func(a rules.Action) error {
		_, err := m.Create(context.Background(), s, rules.CreateInput{Name: "Enroller", TriggerEvents: []string{"x"}, Actions: []rules.Action{a}})
		return err
	}

	require.NoError(t, create(rules.Action{Type: rules.ActionEnroll, AutomationID: fixtures.AutomationJobsGreetingID}))
	require.ErrorIs(t, create(rules.Action{Type: rules.ActionEnroll}), rules.ErrInvalid, "no automation")
	require.ErrorIs(t, create(rules.Action{Type: rules.ActionEnroll, AutomationID: fixtures.AutomationGlobexID}), rules.ErrInvalid, "another workspace's")
	assert.ErrorIs(t, create(rules.Action{Type: rules.ActionEnroll, AutomationID: fixtures.AutomationArchivedID}), rules.ErrInvalid, "an archived one")
}
