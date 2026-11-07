package events_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var firstPage = pagination.Params{Page: 1, PageSize: 100}

func TestListLogFiltersByActionAndOrdersNewestFirst(t *testing.T) {
	env := testhelper.Setup(t)

	p, err := events.ListLog(context.Background(), env.DB.Scoped(fixtures.AcmeID), events.LogFilter{Action: "purchase"}, firstPage)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(p.Items), 2)
	assert.Equal(t, len(p.Items), p.TotalItems)
	for _, e := range p.Items {
		assert.Equal(t, "purchase", e.Action)
	}
	created := lo.Map(p.Items, func(e *ent.Event, _ int) time.Time { return e.CreatedAt })
	assert.True(t, sort.SliceIsSorted(created, func(i, j int) bool { return created[i].After(created[j]) }), "newest first")
}

func TestListLogBreaksCreatedAtTiesByIDDescending(t *testing.T) {
	env := testhelper.Setup(t)

	p, err := events.ListLog(context.Background(), env.DB.Scoped(fixtures.AcmeID), events.LogFilter{Action: "tie_probe"}, firstPage)
	require.NoError(t, err)
	require.Len(t, p.Items, 2)
	assert.Equal(t, int64(fixtures.EventTieSecondID), p.Items[0].ID)
	assert.Equal(t, int64(fixtures.EventTieFirstID), p.Items[1].ID)
}

func TestListLogFiltersByContactID(t *testing.T) {
	env := testhelper.Setup(t)
	cid := int64(fixtures.ContactAliceID)

	p, err := events.ListLog(context.Background(), env.DB.Scoped(fixtures.AcmeID), events.LogFilter{ContactID: &cid}, firstPage)
	require.NoError(t, err)
	require.NotEmpty(t, p.Items)
	for _, e := range p.Items {
		assert.Equal(t, &cid, e.ContactID)
	}
}

func TestListLogFiltersByEmailCaseInsensitively(t *testing.T) {
	env := testhelper.Setup(t)

	p, err := events.ListLog(context.Background(), env.DB.Scoped(fixtures.AcmeID), events.LogFilter{Email: "ALICE@Example.com"}, firstPage)
	require.NoError(t, err)
	require.NotEmpty(t, p.Items)
	for _, e := range p.Items {
		require.NotNil(t, e.Email)
		assert.Equal(t, fixtures.ContactAliceEmail, *e.Email)
	}
}

func TestListLogPagesAndStaysInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	all, err := events.ListLog(ctx, env.DB.Scoped(fixtures.AcmeID), events.LogFilter{}, firstPage)
	require.NoError(t, err)
	for _, e := range all.Items {
		assert.Equal(t, int64(fixtures.AcmeID), e.WorkspaceID)
	}

	p, err := events.ListLog(ctx, env.DB.Scoped(fixtures.AcmeID), events.LogFilter{}, pagination.Params{Page: 2, PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, p.Items, 2)
	assert.Equal(t, all.TotalItems, p.TotalItems)
	assert.Equal(t, all.Items[2].ID, p.Items[0].ID)
}

func TestListDistinctActionsPagesTheDistinctActionsAscending(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	all, err := events.DistinctActions(context.Background(), s)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all), 2)

	second, err := events.ListDistinctActions(context.Background(), s, pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{all[1]}, second.Items)
	assert.Equal(t, len(all), second.TotalItems)
	assert.Equal(t, len(all), second.TotalPages)

	beyond, err := events.ListDistinctActions(context.Background(), s, pagination.Params{Page: len(all) + 1, PageSize: 1})
	require.NoError(t, err)
	assert.Empty(t, beyond.Items)
}

func TestDistinctActionsAreDistinctSortedAndWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The fixtures hold many page_view Events: the action is listed once.
	got, err := events.DistinctActions(ctx, env.DB.Scoped(fixtures.AcmeID))
	require.NoError(t, err)
	assert.True(t, sort.StringsAreSorted(got))
	assert.Contains(t, got, "purchase")
	assert.NotContains(t, got, fixtures.EventGlobexAction)
	pageViews := lo.CountBy(got, func(a string) bool { return a == "page_view" })
	assert.Equal(t, 1, pageViews, "distinct")

	other, err := events.DistinctActions(ctx, env.DB.Scoped(fixtures.GlobexID))
	require.NoError(t, err)
	assert.Equal(t, []string{fixtures.EventGlobexAction}, other)
}
