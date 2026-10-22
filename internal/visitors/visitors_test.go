package visitors_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/visitors"
)

// outboxCollected decodes every CollectedEvent on the domain-event outbox.
func outboxCollected(t *testing.T, env *testhelper.TestEnv) []*events.CollectedEvent {
	t.Helper()
	var out []*events.CollectedEvent
	for _, ev := range env.OutboxEvents(t, events.NameCollected) {
		ce, ok := ev.(*events.CollectedEvent)
		require.Truef(t, ok, "got %T", ev)
		out = append(out, ce)
	}
	return out
}

func TestIdentifyCreatesTheContactBindsTheDeviceAndStitchesEarlierEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The fixtures hold an earlier anonymous event from the device, and one from another device.
	const device, otherDevice = fixtures.EventAnonymousFirstVisitorID, fixtures.EventAnonymousSecondVisitorID

	err := visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), visitors.IdentifyInput{
		VisitorID: "  " + device + " ",
		Email:     new("Visitor@Example.com"),
		Traits:    map[string]any{"plan": "pro"},
	})
	require.NoError(t, err)

	c, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email("visitor@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "pro", c.CustomFields["plan"])

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID(device)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, c.ID, lo.FromPtr(v.ContactID), "the device is bound to the contact")

	stitched, err := env.DB.Event.Query().Where(event.VisitorID(device)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, c.ID, lo.FromPtr(stitched.ContactID), "pre-identify behavior is attached")
	other, err := env.DB.Event.Query().Where(event.VisitorID(otherDevice)).Only(ctx)
	require.NoError(t, err)
	assert.Nil(t, other.ContactID, "another device stays anonymous")
}

func TestIdentifyReusesAnExistingContactAndDevice(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	input := visitors.IdentifyInput{VisitorID: "dev-known", Email: lo.ToPtr(fixtures.ContactAliceEmail)}

	require.NoError(t, visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), input))
	require.NoError(t, visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), input), "identify is idempotent")

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-known")).Only(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.ContactAliceID, lo.FromPtr(v.ContactID))
	n, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.Email(fixtures.ContactAliceEmail)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "no duplicate contact")
}

func TestIdentifyRefusesWhatCannotBeIdentified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), visitors.IdentifyInput{VisitorID: "   ", Email: new("a@example.com")})
	require.EqualError(t, err, "visitorId is required")

	err = visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), visitors.IdentifyInput{VisitorID: "dev-x"})
	require.EqualError(t, err, "identify requires subjectId, email, or phone")
	n, err := env.DB.Visitor.Query().Where(visitor.VisitorID("dev-x")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a refused identify leaves no device behind")
}

func TestCollectCarriesTheIdentityTheDeviceResolvesTo(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	at := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	require.NoError(t, visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), visitors.IdentifyInput{
		VisitorID: "dev-id", Email: lo.ToPtr(fixtures.ContactAliceEmail), SubjectID: new("alice-1"),
	}))

	require.NoError(t, visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), []visitors.CollectEventInput{
		{VisitorID: " dev-id ", Action: "signup", Properties: map[string]any{"plan": "pro"}, OccurredAt: &at},
		{VisitorID: "dev-anon", Action: "page_view"},
	}))

	got := outboxCollected(t, env)
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
	require.NoError(t, visitors.Identify(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), visitors.IdentifyInput{VisitorID: "dev-gone", Email: new("gone@example.com")}))
	_, err := env.DB.Contact.Delete().Where(contact.Email("gone@example.com")).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, visitors.Collect(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), []visitors.CollectEventInput{{VisitorID: "dev-gone", Action: "login"}}))
	got := outboxCollected(t, env)
	require.Len(t, got, 1)
	assert.Zero(t, got[0].ContactID)
}
