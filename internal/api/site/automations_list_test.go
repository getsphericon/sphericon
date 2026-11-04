package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The list searches the whole collection by name, ignoring case, keeps one state (Draft until the
// first publish, then Active or Inactive) and orders by creation, the newest first, or by name, A
// to Z, each way round on request.
func TestSiteAutomationsListFiltersByNameAndStateAndSorts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	create := func(name, trigger string) siteapi.EntityId {
		t.Helper()
		res, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
			Name:         name,
			TriggerEvent: trigger,
			Steps:        []siteapi.SiteAutomationStep{{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(60)}},
		}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
		require.NoError(t, err)
		created, ok := res.(*siteapi.SiteAutomationResource)
		require.Truef(t, ok, "got %T", res)
		return created.ID
	}
	publish := func(id siteapi.EntityId) {
		t.Helper()
		_, err := c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: workspaceID, ID: id})
		require.NoError(t, err)
	}
	draft := create("List probe Alpha", "list_probe_alpha")
	active := create("List probe Beta", "list_probe_beta")
	publish(active)
	inactive := create("List probe Gamma", "list_probe_gamma")
	publish(inactive)
	_, err := c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: workspaceID, ID: inactive})
	require.NoError(t, err)

	list := func(params siteapi.SiteAutomationsListParams) []siteapi.EntityId {
		t.Helper()
		params.WorkspaceId = workspaceID
		params.Name = siteapi.NewOptString("LIST PROBE")
		res, err := c.SiteAutomationsList(ctx, params)
		require.NoError(t, err)
		page, ok := res.(*siteapi.SiteAutomationsListOK)
		require.Truef(t, ok, "got %T", res)
		ids := lo.Map(page.Items, func(a siteapi.SiteAutomationResource, _ int) siteapi.EntityId { return a.ID })
		assert.EqualValues(t, len(ids), page.TotalItems, "the total counts what the filter keeps")
		return ids
	}

	assert.Equal(t, []siteapi.EntityId{inactive, active, draft}, list(siteapi.SiteAutomationsListParams{}),
		"the name filter ignores case, and the newest comes first")
	assert.Equal(t, []siteapi.EntityId{draft, active, inactive}, list(siteapi.SiteAutomationsListParams{
		Direction: siteapi.NewOptSortDirection(siteapi.SortDirectionAsc),
	}))
	assert.Equal(t, []siteapi.EntityId{draft, active, inactive}, list(siteapi.SiteAutomationsListParams{
		Sort: siteapi.NewOptSiteAutomationsSort(siteapi.SiteAutomationsSortName),
	}), "by name the default is A to Z")
	assert.Equal(t, []siteapi.EntityId{inactive, active, draft}, list(siteapi.SiteAutomationsListParams{
		Sort:      siteapi.NewOptSiteAutomationsSort(siteapi.SiteAutomationsSortName),
		Direction: siteapi.NewOptSortDirection(siteapi.SortDirectionDesc),
	}))

	for state, want := range map[siteapi.SiteAutomationState]siteapi.EntityId{
		siteapi.SiteAutomationStateDraft:    draft,
		siteapi.SiteAutomationStateActive:   active,
		siteapi.SiteAutomationStateInactive: inactive,
	} {
		assert.Equal(t, []siteapi.EntityId{want}, list(siteapi.SiteAutomationsListParams{
			State: siteapi.NewOptSiteAutomationState(state),
		}), state)
	}
}
