package collect_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/visitor"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/collect"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A property keeps its JSON value: a string, a number, a boolean, an object, a list and null
// arrive in the collected Event as the values they were sent as.
func TestCollectedEventPropertiesKeepTheirJSONValues(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.CollectAcme(t).CollectEventsCreate(context.Background(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{
			VisitorId: "dev-props",
			Action:    "viewed",
			Properties: collectapi.NewOptNilCollectEventInputProperties(collectapi.CollectEventInputProperties{
				"s": jx.Raw(`"x"`), "n": jx.Raw(`2`), "b": jx.Raw(`true`), "o": jx.Raw(`{"k":"v"}`), "l": jx.Raw(`[1]`), "z": jx.Raw(`null`),
			}),
		}},
	})
	require.NoError(t, err)
	require.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)

	collected := env.OutboxEvents(t, events.NameCollected)
	require.Len(t, collected, 1)
	got, ok := collected[0].(*events.CollectedEvent)
	require.Truef(t, ok, "got %T", collected[0])
	assert.Equal(t, map[string]any{
		"s": "x", "n": float64(2), "b": true, "o": map[string]any{"k": "v"}, "l": []any{float64(1)}, "z": nil,
	}, got.Properties)
}

func TestCollectIdentifyAssertsPhoneAndSubjectID(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()

	res, err := c.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "dev-phone",
		Phone:     collectapi.NewOptNilString("+15553331"),
		SubjectId: collectapi.NewOptNilString("crm-77"),
	})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectOkResponse{}, res)

	ct, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), contact.SubjectID("crm-77")).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, ct.Phone)
	assert.Equal(t, "+15553331", *ct.Phone)

	v, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-phone")).Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, v.ContactID)
	assert.Equal(t, ct.ID, *v.ContactID)
}

// The write key decides the tenant: the same device id under another workspace's key
// identifies that workspace's contact only.
func TestCollectIdentifyIsScopedToTheKeysWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	res, err := env.CollectWithKey(t, fixtures.GlobexCollectKey).CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "shared-device",
		Email:     collectapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail),
	})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectOkResponse{}, res)

	globexAlice, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.GlobexID), contact.Email(fixtures.ContactAliceEmail)).Only(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, int64(fixtures.ContactAliceID), globexAlice.ID, "Acme's Alice is untouched")
	n, err := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("shared-device")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestCollectIdentifyWithoutAnAliasKeyIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.CollectAcme(t).CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{VisitorId: "dev-anon-only"})
	require.Error(t, err, "nothing to identify the visitor by")

	n, err := env.DB.Visitor.Query().Where(visitor.VisitorID("dev-anon-only")).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "a refused identify leaves no device behind")

	_, err = env.CollectAcme(t).CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "   ", Email: collectapi.NewOptNilEmailAddress("blank@example.com"),
	})
	require.Error(t, err, "a blank visitor id")
	exists, err := env.DB.Contact.Query().Where(contact.Email("blank@example.com")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCollectEventsCarryOccurredAtPropertiesAndTheKeysWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	at := time.Date(2026, 4, 2, 10, 30, 0, 0, time.UTC)

	res, err := env.CollectWithKey(t, fixtures.GlobexCollectKey).CollectEventsCreate(ctx, &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{
			VisitorId:  "dev-ev",
			Action:     "checkout",
			OccurredAt: collectapi.NewOptNilTimestamp(collectapi.Timestamp(at)),
			Properties: collectapi.NewOptNilCollectEventInputProperties(collectapi.CollectEventInputProperties{"total": jx.Raw(`12.5`)}),
		}},
	})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)

	collected := env.OutboxEvents(t, events.NameCollected)
	require.Len(t, collected, 1, "exactly one event")
	decoded := collected[0]
	got, ok := decoded.(*events.CollectedEvent)
	require.Truef(t, ok, "got %T", decoded)

	assert.EqualValues(t, fixtures.GlobexID, got.WorkspaceID)
	assert.Equal(t, "checkout", got.Action)
	assert.True(t, got.OccurredAt.Equal(at))
	assert.InDelta(t, 12.5, got.Properties["total"], 0.001)
	assert.Zero(t, got.ContactID, "an unidentified device is anonymous")
}

// A storage outage is an error to the caller (so the tracker retries), never an
// acknowledged ingest.
func TestCollectEventsStorageFailureIsNotAcknowledged(t *testing.T) {
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	h := collect.NewHandlers(events.New(sqlDB))

	res, err := h.CollectEventsCreate(
		auth.WithCollectAuth(context.Background(), &auth.CollectAuth{WorkspaceID: fixtures.AcmeID, Scoped: db.NewEntClient(sqlDB).Scoped(fixtures.AcmeID)}),
		&collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{{VisitorId: "dev", Action: "x"}}},
	)
	require.Error(t, err)
	assert.Nil(t, res)
}
