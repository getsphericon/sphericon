package visitors_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/visitoridentification"
)

// queuedCollected decodes every CollectedEvent the test published.
func queuedCollected(t *testing.T, env *testhelper.TestEnv) []*events.CollectedEvent {
	t.Helper()
	return lo.Map(env.QueuedEvents(t, events.NameCollected), func(ev events.DomainEvent, _ int) *events.CollectedEvent {
		ce, ok := ev.(*events.CollectedEvent)
		require.Truef(t, ok, "got %T", ev)
		return ce
	})
}

func TestCollectCarriesTheIdentityTheDeviceResolvesTo(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	require.NoError(t, visitoridentification.New(env.Bus).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
		VisitorID: "dev-id", Email: lo.ToPtr(fixtures.ContactAliceEmail), SubjectID: new("alice-1"),
	}))

	require.NoError(t, visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), []visitors.CollectEventInput{
		{VisitorID: " dev-id ", Action: "signup", Properties: map[string]any{"plan": "pro"}, OccurredAt: &at},
		{VisitorID: "dev-anon", Action: "page_view"},
	}))

	got := queuedCollected(t, env)
	require.Len(t, got, 2)
	known, anon := got[0], got[1]
	assert.Equal(t, "signup", known.Action)
	assert.EqualValues(t, fixtures.ContactAliceID, known.ContactID)
	assert.Equal(t, fixtures.ContactAliceEmail, known.Email)
	assert.Equal(t, "alice-1", known.SubjectID)
	assert.Equal(t, "dev-id", known.VisitorID)
	assert.Equal(t, "pro", known.Properties["plan"])
	assert.True(t, known.OccurredAt.Equal(at))

	assert.Equal(t, "page_view", anon.Action)
	assert.Zero(t, anon.ContactID, "an unidentified device is anonymous")
	assert.Equal(t, "dev-anon", anon.VisitorID)
	assert.True(t, anon.OccurredAt.IsZero())

	n, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-anon")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "an unseen device is recorded on first event")
}

func TestCollectTreatsAVanishedContactAsAnonymous(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.StarkID), []visitors.CollectEventInput{{VisitorID: fixtures.VisitorStarkVanishedVisitorID, Action: "login"}}))
	got := queuedCollected(t, env)
	require.Len(t, got, 1)
	assert.Zero(t, got[0].ContactID)
}
