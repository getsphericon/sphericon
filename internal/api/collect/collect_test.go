package collect_test

import (
	"context"
	"testing"

	"github.com/go-faster/jx"
	"github.com/mokevnin/sphericon/ent/contact"
	"github.com/mokevnin/sphericon/ent/customfield"
	"github.com/mokevnin/sphericon/ent/event"
	collectapi "github.com/mokevnin/sphericon/gen/collect"
	"github.com/mokevnin/sphericon/internal/events"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/segments"
	"github.com/mokevnin/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectKeyAuth(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := env.CollectAnonymous(t).CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateUnauthorized{}, res)

	res, err = env.CollectWithKey(t, "wrong-key").CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateUnauthorized{}, res)
}

func TestCollectIdentifyAndEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()

	// Identify with typed traits — exercises map[string]jx.Raw -> map[string]any.
	ident, err := c.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "v1",
		Email:     collectapi.NewOptNilEmailAddress("trav@example.com"),
		Traits: collectapi.NewOptNilCollectIdentifyInputTraits(collectapi.CollectIdentifyInputTraits{
			"plan":   jx.Raw(`"pro"`),
			"visits": jx.Raw(`3`),
		}),
	})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectOkResponse{}, ident)

	// Identify upserts a Contact (not a separate tracking profile) and merges the
	// traits as typed custom field values.
	ct, err := env.DB.Contact.Query().
		Where(contact.Email("trav@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "pro", ct.CustomFields["plan"])
	assert.Equal(t, float64(3), ct.CustomFields["visits"])  // JSON numbers → float64
	assert.Equal(t, int64(fixtures.AcmeID), ct.WorkspaceID) // scoped to the key's workspace

	// Each trait key auto-created a typed CustomField definition (declared-by-use).
	planDef, err := env.DB.CustomField.Query().
		Where(customfield.WorkspaceID(fixtures.AcmeID), customfield.Key("plan")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, customfield.TypeString, planDef.Type)
	visitsDef, err := env.DB.CustomField.Query().
		Where(customfield.WorkspaceID(fixtures.AcmeID), customfield.Key("visits")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, customfield.TypeNumber, visitsDef.Type)

	// Events with properties — same conversion path.
	evRes, err := c.CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{
			VisitorId: "v1",
			Action:    "page_view",
			Properties: collectapi.NewOptNilCollectEventInputProperties(collectapi.CollectEventInputProperties{
				"url": jx.Raw(`"/pricing"`),
				"n":   jx.Raw(`5`),
			}),
		}},
	})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, evRes)

	// The event is published to the transactional outbox as a CollectedEvent (the
	// persist subscriber writes the Event row asynchronously; the router isn't run
	// under txdb). Decode the outbox row and assert the customer's event is carried
	// as-is, with the resolved identity.
	outbox := env.OutboxEnvelopes(t)
	require.NotEmpty(t, outbox)
	envlp := outbox[len(outbox)-1]
	assert.Equal(t, events.NameCollected, envlp.Name)
	assert.EqualValues(t, fixtures.AcmeID, envlp.WorkspaceID) // scoped to the key's workspace

	decoded, err := events.Decode(envlp)
	require.NoError(t, err)
	ce, ok := decoded.(*events.CollectedEvent)
	require.Truef(t, ok, "got %T", decoded)
	assert.Equal(t, "page_view", ce.Action)
	assert.Equal(t, "trav@example.com", ce.Email)
	// Visitor v1 was bound to the Contact at Identify, so the event resolves to it.
	assert.Equal(t, ct.ID, ce.ContactID)
	assert.Equal(t, "/pricing", ce.Properties["url"])
	assert.Equal(t, float64(5), ce.Properties["n"])
}

// The headline ADR 0002 behavior: an event recorded BEFORE Identify (anonymous,
// contact_id null) is stitched onto the resolved Contact at Identify, which makes it
// visible to an event-based segment condition that keys on the stable contact_id.
func TestCollectIdentifyStitchesAnonymousEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()

	// A pre-identify anonymous event from device "vX" (as the persist subscriber
	// would have written it: visitor_id set, contact_id null).
	_, err := env.DB.Event.Create().
		SetWorkspaceID(fixtures.AcmeID).SetVisitorID("vX").SetAction("page_view").Save(ctx)
	require.NoError(t, err)

	// Identify the device → binds it to a Contact and backfills its anonymous events.
	_, err = c.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "vX",
		Email:     collectapi.NewOptNilEmailAddress("stitch@example.com"),
	})
	require.NoError(t, err)

	ct, err := env.DB.Contact.Query().Where(contact.Email("stitch@example.com")).Only(ctx)
	require.NoError(t, err)

	// The previously anonymous event is now attached to the Contact (the stitch UPDATE
	// actually ran — not a no-op).
	ev, err := env.DB.Event.Query().Where(event.VisitorID("vX")).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, ev.ContactID)
	assert.Equal(t, ct.ID, *ev.ContactID)

	// End-to-end: the stitched event makes the Contact match an event-condition
	// segment (which joins Contact↔Event on contact_id, per ADR 0002).
	pred, err := segments.ContactPredicate(ctx, env.DB.Scoped(fixtures.AcmeID),
		`{"combinator":"and","rules":[{"field":"event:page_view","operator":"performed","value":""}]}`,
	)
	require.NoError(t, err)
	n, err := env.DB.Contact.Query().Where(contact.IDEQ(ct.ID), pred).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "stitched anonymous event must make the contact match the segment")
}
