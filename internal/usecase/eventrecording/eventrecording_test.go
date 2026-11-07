package eventrecording_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/eventrecording"
)

// Fixtures: workspace acme (1) has events page_view and purchase; contact 1 is
// alice@example.com. Assertions read what Record published, decoded.

func queuedCollected(t *testing.T, env *testhelper.TestEnv) []*events.CollectedEvent {
	t.Helper()
	return lo.Map(env.QueuedEvents(t), func(ev events.DomainEvent, _ int) *events.CollectedEvent {
		ce, ok := ev.(*events.CollectedEvent)
		require.Truef(t, ok, "got %T", ev)
		return ce
	})
}

func TestRecordAttachesEventToExistingContactByAlias(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	err := m.Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "s-1", Action: "page_view", Email: new("ALICE@example.com")},
		{SubjectID: "nobody", Action: "signup", Email: new("nobody@example.com")},
	})
	require.NoError(t, err)

	got := queuedCollected(t, env)
	require.Len(t, got, 2)
	assert.Equal(t, int64(fixtures.ContactAliceID), got[0].ContactID, "email alias resolves to the existing contact")
	assert.Equal(t, int64(0), got[1].ContactID, "unknown identity stays anonymous")
	assert.Equal(t, int64(fixtures.AcmeID), got[0].WorkspaceID)
	assert.Equal(t, "page_view", got[0].Action)
}

func TestRecordCarriesPropertiesVerbatim(t *testing.T) {
	env := testhelper.Setup(t)
	m := newModule(env)

	require.NoError(t, m.Record(context.Background(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "s", Action: "added_to_cart", Properties: map[string]any{"sku": "A1"}},
	}))

	got := queuedCollected(t, env)
	require.Len(t, got, 1)
	assert.Equal(t, "A1", got[0].Properties["sku"])
}

// liveWindow is the test instance's live window, the production default; testNow is its clock.
const liveWindow = 72 * time.Hour

var testNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func newModule(env *testhelper.TestEnv) *eventrecording.UseCase {
	return eventrecording.New(env.Bus, eventrecording.LivePolicy{Window: liveWindow, Now: func() time.Time { return testNow }})
}
