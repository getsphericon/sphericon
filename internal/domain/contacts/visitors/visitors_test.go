package visitors_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/visitoridentification"
)

// queuedCollected decodes every CollectedEvent the test published.
func queuedCollected(t *testing.T, env *testhelper.TestEnv) []*events.CollectedEvent {
	t.Helper()
	return testhelper.Queued[*events.CollectedEvent](t, env, events.NameCollected)
}

func TestCollectCarriesTheIdentityTheDeviceResolvesTo(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	// A claim gives a Contact that exists no alias key (ADR 0171): Alice holds hers already.
	require.NoError(t, env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).SetSubjectID("alice-1").Exec(ctx))

	require.NoError(t, testhelper.Module[*visitoridentification.UseCase](env).Identify(ctx, env.DB.Scoped(fixtures.AcmeID), visitoridentification.Input{
		VisitorID: "dev-id", Email: lo.ToPtr(fixtures.ContactAliceEmail), SubjectID: new("alice-1"),
	}))

	require.NoError(t, visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), []visitors.CollectEventInput{
		{VisitorID: " dev-id ", Action: "signup", Properties: map[string]any{"plan": "pro"}, OccurredAt: &at},
		{VisitorID: "dev-anon", Action: "page_view"},
	}, time.Now()))

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

// Deleting a Contact clears its Visitors' contact_id (ON DELETE SET NULL), so the device of a
// deleted Contact is anonymous again.
func TestCollectTreatsTheDeviceOfADeletedContactAsAnonymous(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.StarkID), []visitors.CollectEventInput{{VisitorID: fixtures.VisitorStarkVanishedVisitorID, Action: "login"}}, time.Now()))
	got := queuedCollected(t, env)
	require.Len(t, got, 1)
	assert.Zero(t, got[0].ContactID)
}

// A collected event outside the contract's bounds (its visitor id, action or client event id)
// refuses the whole batch, whichever driver collects it: no event of it is accepted.
func TestCollectRefusesABatchWithAnEventOutsideTheContractsBounds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	ok := visitors.CollectEventInput{VisitorID: "dev-bounded", Action: "login"}

	for name, bad := range map[string]visitors.CollectEventInput{
		"a blank visitor id":         {VisitorID: " ", Action: "login"},
		"a visitor id too long":      {VisitorID: strings.Repeat("v", 256), Action: "login"},
		"no action":                  {VisitorID: "dev-bounded"},
		"an action too long":         {VisitorID: "dev-bounded", Action: strings.Repeat("a", 256)},
		"a client event id too long": {VisitorID: "dev-bounded", Action: "login", ClientEventID: strings.Repeat("c", 256)},
	} {
		err := visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), []visitors.CollectEventInput{ok, bad}, time.Now())

		refusal, isRefusal := errors.AsType[*validation.RefusalError](err)
		require.True(t, isRefusal, "%s: %v", name, err)
		assert.Equal(t, validation.KindRefused, refusal.Kind(), name)
	}
	assert.Empty(t, queuedCollected(t, env), "no event of a refused batch is accepted")
}
