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
// to Z, each way round on request. Duff has one of each state, created Alpha, Beta, Gamma.
func TestSiteAutomationsListFiltersByNameAndStateAndSorts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DuffOwnerDanEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.DuffPublicID)
	draft := idStr(fixtures.AutomationDuffDraftID)
	active := idStr(fixtures.AutomationDuffActiveID)
	inactive := idStr(fixtures.AutomationDuffInactiveID)

	list := func(params siteapi.SiteAutomationsListParams) []siteapi.EntityId {
		t.Helper()
		params.WorkspaceId = workspaceID
		params.Name = siteapi.NewOptString("LIST PROBE")
		res, err := c.SiteAutomationsList(ctx, params)
		require.NoError(t, err)
		page := res
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
