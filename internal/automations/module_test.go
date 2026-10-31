package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestModuleIsConfinedToTheScopedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	ctx := context.Background()
	acme, globex := env.DB.Scoped(fixtures.AcmeID), env.DB.Scoped(fixtures.GlobexID)

	_, err := m.Get(ctx, acme, fixtures.AutomationGlobexID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	_, err = m.Get(ctx, globex, fixtures.AutomationWelcomeSeriesID)
	require.ErrorIs(t, err, automations.ErrNotFound)

	_, err = m.Publish(ctx, acme, fixtures.AutomationGlobexID)
	require.ErrorIs(t, err, automations.ErrNotFound)
	require.ErrorIs(t, m.Delete(ctx, acme, fixtures.AutomationGlobexID), automations.ErrNotFound)

	created, err := m.Create(ctx, acme, automations.CreateInput{Name: "New", TriggerEvent: "contact.created"})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, created.WorkspaceID)

	page, err := m.List(ctx, globex, pagination.Params{Page: 1, PageSize: 100}, false)
	require.NoError(t, err)
	for _, a := range page.Items {
		assert.EqualValues(t, fixtures.GlobexID, a.WorkspaceID)
	}
}
