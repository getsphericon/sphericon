package eventlog_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures: workspace acme (1) has events page_view and purchase; contact 1 is
// alice@example.com. Ingest publishes to the outbox (the persist subscriber is not
// running under txdb), so assertions read the decoded outbox rows.

func outboxCollected(t *testing.T, env *testhelper.TestEnv) []*events.CollectedEvent {
	t.Helper()
	var out []*events.CollectedEvent
	for _, ev := range env.OutboxEvents(t) {
		ce, ok := ev.(*events.CollectedEvent)
		require.Truef(t, ok, "got %T", ev)
		out = append(out, ce)
	}
	return out
}

func TestIngestAttachesEventToExistingContactByAlias(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	err := m.Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{
		{SubjectID: "s-1", Action: "page_view", Email: new("ALICE@example.com")},
		{SubjectID: "nobody", Action: "signup", Email: new("nobody@example.com")},
	})
	require.NoError(t, err)

	got := outboxCollected(t, env)
	require.Len(t, got, 2)
	assert.Equal(t, int64(fixtures.ContactAliceID), got[0].ContactID, "email alias resolves to the existing contact")
	assert.Equal(t, int64(0), got[1].ContactID, "unknown identity stays anonymous")
	assert.Equal(t, int64(fixtures.AcmeID), got[0].WorkspaceID)
	assert.Equal(t, "page_view", got[0].Action)
}

func TestIngestCarriesPropertiesVerbatim(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	require.NoError(t, m.Ingest(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventlog.Input{
		{SubjectID: "s", Action: "added_to_cart", Properties: map[string]any{"sku": "A1"}},
	}))

	got := outboxCollected(t, env)
	require.Len(t, got, 1)
	assert.Equal(t, "A1", got[0].Properties["sku"])
}

func TestActionsAreDistinctSortedAndWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := newModule(env)

	env.DB.Event.Create().SetWorkspaceID(fixtures.AcmeID).SetSubjectID("dup").SetAction("page_view").SaveX(ctx)

	got, err := m.Actions(ctx, env.DB.Scoped(fixtures.AcmeID))
	require.NoError(t, err)
	assert.True(t, sort.StringsAreSorted(got))
	assert.Contains(t, got, "purchase")
	assert.NotContains(t, got, fixtures.EventGlobexAction)
	pageViews := lo.CountBy(got, func(a string) bool { return a == "page_view" })
	assert.Equal(t, 1, pageViews, "distinct")

	other, err := m.Actions(ctx, env.DB.Scoped(fixtures.GlobexID))
	require.NoError(t, err)
	assert.Equal(t, []string{fixtures.EventGlobexAction}, other)
}

// liveWindow is the test instance's live window, the production default; testNow is its clock.
const liveWindow = 72 * time.Hour

var testNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func newModule(env *testhelper.TestEnv) *eventlog.Module {
	return eventlog.New(env.Bus, eventlog.LivePolicy{Window: liveWindow, Now: func() time.Time { return testNow }})
}
