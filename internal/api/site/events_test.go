package site_test

import (
	"context"
	"sort"
	"testing"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteEventsListMostRecentFirst(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteEventsList(context.Background(), siteapi.SiteEventsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	ok, isOK := res.(*siteapi.SiteEventsListOK)
	require.Truef(t, isOK, "got %T", res)

	// The page is ordered most-recent-first: each item's createdAt is >= the next.
	require.NotEmpty(t, ok.Items)
	for i := 1; i < len(ok.Items); i++ {
		assert.Falsef(t, time.Time(ok.Items[i-1].CreatedAt).Before(time.Time(ok.Items[i].CreatedAt)),
			"events must be newest-first, but item %d is older than item %d", i-1, i)
	}
}

func TestSiteEventsListFilterByAction(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteEventsList(context.Background(), siteapi.SiteEventsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		Action:      siteapi.NewOptString("purchase"),
	})
	require.NoError(t, err)
	ok, isOK := res.(*siteapi.SiteEventsListOK)
	require.Truef(t, isOK, "got %T", res)

	// The action filter selects only matching events.
	require.NotEmpty(t, ok.Items)
	for _, it := range ok.Items {
		assert.Equal(t, "purchase", it.Action)
	}
}

func TestSiteEventsListFilterByEmail(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	// Upper-cased on purpose: the match must be case-insensitive because contact
	// emails are stored as entered while collect events are lowercased.
	res, err := c.SiteEventsList(context.Background(), siteapi.SiteEventsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		Email:       siteapi.NewOptString("ALICE@example.com"),
	})
	require.NoError(t, err)
	ok, isOK := res.(*siteapi.SiteEventsListOK)
	require.Truef(t, isOK, "got %T", res)

	// The email filter selects only alice's events (case-insensitively); bob's are
	// excluded. Alice's single seeded event is her page_view.
	require.NotEmpty(t, ok.Items)
	for _, it := range ok.Items {
		require.True(t, it.Email.Set)
		assert.Equal(t, fixtures.ContactAliceEmail, it.Email.Value)
	}
	assert.Equal(t, "page_view", ok.Items[0].Action)
}

func TestSiteEventsListUnknownWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteEventsList(context.Background(), siteapi.SiteEventsListParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

// SiteEventsActions returns the workspace's distinct actions, sorted — the
// segment builder uses it to populate event conditions. A duplicate page_view
// must still appear once.
func TestSiteEventsActions(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Many fixture events are page_views: de-duplication shows it once.
	out, err := c.SiteEventsActions(ctx, siteapi.SiteEventsActionsParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res, ok := out.(*siteapi.SiteEventActionsResult)
	require.Truef(t, ok, "got %T", out)

	assert.True(t, sort.StringsAreSorted(res.Actions), "actions are sorted")
	seen := map[string]int{}
	for _, a := range res.Actions {
		seen[a]++
	}
	assert.Equal(t, 1, seen["page_view"], "distinct: page_view appears once despite duplicates")
	assert.Contains(t, res.Actions, "purchase", "known seeded action is present")
}

func TestSiteEventsActionsUnknownWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteEventsActions(context.Background(), siteapi.SiteEventsActionsParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}
