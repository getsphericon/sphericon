package eventlog_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var firstPage = pagination.Params{Page: 1, PageSize: 100}

func TestListFiltersByActionAndOrdersNewestFirst(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	p, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), eventlog.Filter{Action: "purchase"}, firstPage)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(p.Items), 2)
	assert.Equal(t, len(p.Items), p.TotalItems)
	for _, e := range p.Items {
		assert.Equal(t, "purchase", e.Action)
	}
	created := lo.Map(p.Items, func(e *ent.Event, _ int) time.Time { return e.CreatedAt })
	assert.True(t, sort.SliceIsSorted(created, func(i, j int) bool { return created[i].After(created[j]) }), "newest first")
}

func TestListBreaksCreatedAtTiesByIDDescending(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	p, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), eventlog.Filter{Action: "tie_probe"}, firstPage)
	require.NoError(t, err)
	require.Len(t, p.Items, 2)
	assert.Equal(t, int64(fixtures.EventTieSecondID), p.Items[0].ID)
	assert.Equal(t, int64(fixtures.EventTieFirstID), p.Items[1].ID)
}

func TestListFiltersByContactID(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	cid := int64(fixtures.ContactAliceID)

	p, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), eventlog.Filter{ContactID: &cid}, firstPage)
	require.NoError(t, err)
	require.NotEmpty(t, p.Items)
	for _, e := range p.Items {
		assert.Equal(t, &cid, e.ContactID)
	}
}

func TestListFiltersByEmailCaseInsensitively(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	p, err := m.List(context.Background(), env.DB.Scoped(fixtures.AcmeID), eventlog.Filter{Email: "ALICE@Example.com"}, firstPage)
	require.NoError(t, err)
	require.NotEmpty(t, p.Items)
	for _, e := range p.Items {
		require.NotNil(t, e.Email)
		assert.Equal(t, fixtures.ContactAliceEmail, *e.Email)
	}
}

func TestListPagesAndStaysInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	ctx := context.Background()

	all, err := m.List(ctx, env.DB.Scoped(fixtures.AcmeID), eventlog.Filter{}, firstPage)
	require.NoError(t, err)
	for _, e := range all.Items {
		assert.Equal(t, int64(fixtures.AcmeID), e.WorkspaceID)
	}

	p, err := m.List(ctx, env.DB.Scoped(fixtures.AcmeID), eventlog.Filter{}, pagination.Params{Page: 2, PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, p.Items, 2)
	assert.Equal(t, all.TotalItems, p.TotalItems)
	assert.Equal(t, all.Items[2].ID, p.Items[0].ID)
}

func TestListActionsPagesTheDistinctActionsAscending(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)
	s := env.DB.Scoped(fixtures.AcmeID)

	all, err := m.Actions(context.Background(), s)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all), 2)

	second, err := m.ListActions(context.Background(), s, pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{all[1]}, second.Items)
	assert.Equal(t, len(all), second.TotalItems)
	assert.Equal(t, len(all), second.TotalPages)

	beyond, err := m.ListActions(context.Background(), s, pagination.Params{Page: len(all) + 1, PageSize: 1})
	require.NoError(t, err)
	assert.Empty(t, beyond.Items)
}
