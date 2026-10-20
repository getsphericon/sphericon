package external_test

import (
	"context"
	"testing"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventsNow is the clock of the tests that judge an Event's age against the live window.
var eventsNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func eventsClock() time.Time { return eventsNow }

func contactsCreatedFor(t *testing.T, env *testhelper.TestEnv, email string) int {
	t.Helper()
	return env.OutboxCount(t, "contact.created", map[string]any{"email": email})
}

// A batch mixing new, existing and unidentifiable contacts reports each item and
// lets the valid ones through; only new contacts publish contact.created.
func TestExternalContactsBatchUpsertMixedItems(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsBatchUpsert(context.Background(), &externalapi.UpsertContactsInput{
		Contacts: []externalapi.UpsertContactInput{
			{Email: externalapi.NewOptNilEmailAddress("batch-new@example.com"), FirstName: externalapi.NewOptNilString("New")},
			{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
			{FirstName: externalapi.NewOptNilString("Nobody")}, // no alias key
			{SubjectId: externalapi.NewOptNilString("batch-subject-1")},
		},
	})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.UpsertContactsResult)
	require.Truef(t, isOK, "got %T", res)
	require.Len(t, ok.Results, 4)

	for i, r := range ok.Results {
		assert.EqualValues(t, i, r.Index)
	}
	assert.Equal(t, externalapi.ContactBatchStatusCreated, ok.Results[0].Status)
	assert.True(t, ok.Results[0].ContactId.IsSet())
	assert.Equal(t, externalapi.ContactBatchStatusUpdated, ok.Results[1].Status)
	id, _ := ok.Results[1].ContactId.Get()
	assert.Equal(t, entityIDString(fixtures.ContactAliceID), id)
	assert.Equal(t, externalapi.ContactBatchStatusFailed, ok.Results[2].Status)
	assert.NotEmpty(t, ok.Results[2].Error.Or(""))
	assert.False(t, ok.Results[2].ContactId.IsSet())
	assert.Equal(t, externalapi.ContactBatchStatusCreated, ok.Results[3].Status)

	assert.Equal(t, 1, contactsCreatedFor(t, env, "batch-new@example.com"))
	assert.Equal(t, 0, contactsCreatedFor(t, env, fixtures.ContactAliceEmail))
}

func TestExternalContactsBatchUpsertLimitAndScope(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	ctx := context.Background()

	writer := env.ExternalScoped(t, "contacts:write")
	tooMany := make([]externalapi.UpsertContactInput, 1001)
	for i := range tooMany {
		tooMany[i] = externalapi.UpsertContactInput{SubjectId: externalapi.NewOptNilString("limit")}
	}
	_, err := writer.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{Contacts: tooMany})
	require.Error(t, err, "more than 1000 items is rejected as a whole")

	reader := env.ExternalScoped(t, "contacts:read")
	res, err := reader.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{
		Contacts: []externalapi.UpsertContactInput{{SubjectId: externalapi.NewOptNilString("x")}},
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsBatchUpsertUnauthorized{}, res)
}

func TestExternalEventsBatchSubmitMixedItems(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	res, err := c.EventsBatchSubmit(context.Background(), &externalapi.RecordEventsBatchInput{
		Events: []externalapi.EventInput{
			{SubjectId: "user:" + fixtures.ContactAliceEmail, Action: "batch_a"},
			{SubjectId: "user:" + fixtures.ContactAliceEmail, Action: "  "}, // blank action
			{SubjectId: "", Action: "batch_c"},                              // blank subject
			{SubjectId: "user:" + fixtures.ContactBobEmail, Action: "batch_d"},
		},
	})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.RecordEventsBatchResult)
	require.Truef(t, isOK, "got %T", res)
	require.Len(t, ok.Results, 4)

	want := []externalapi.EventBatchStatus{
		externalapi.EventBatchStatusAccepted, externalapi.EventBatchStatusFailed,
		externalapi.EventBatchStatusFailed, externalapi.EventBatchStatusAccepted,
	}
	for i, r := range ok.Results {
		assert.EqualValues(t, i, r.Index)
		assert.Equal(t, want[i], r.Status)
		if r.Status == externalapi.EventBatchStatusFailed {
			assert.NotEmpty(t, r.Error.Or(""))
		}
	}

	var actions []string
	for _, r := range outboxCollected(t, env) {
		actions = append(actions, r.event.Action)
	}
	assert.ElementsMatch(t, []string{"batch_a", "batch_d"}, actions, "only the valid items were published")
}

func TestExternalEventsBatchSubmitScope(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:read")
	res, err := c.EventsBatchSubmit(context.Background(), &externalapi.RecordEventsBatchInput{
		Events: []externalapi.EventInput{{SubjectId: "u", Action: "a"}},
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EventsBatchSubmitUnauthorized{}, res)
}

// A backfilled batch is accepted per item and published as history; a client event id rides along as
// the dedup key.
func TestExternalEventsBatchBackfill(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	res, err := c.EventsBatchSubmit(context.Background(), &externalapi.RecordEventsBatchInput{
		Backfill: externalapi.NewOptBool(true),
		Events: []externalapi.EventInput{
			{
				SubjectId: "s-1", Action: "order_paid", ClientEventId: externalapi.NewOptNilString("order-1"),
				OccurredAt: externalapi.NewOptNilTimestamp(externalapi.Timestamp(eventsNow.AddDate(-2, 0, 0))),
			},
		},
	})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.RecordEventsBatchResult)
	require.Truef(t, isOK, "got %T", res)
	require.Len(t, ok.Results, 1)
	assert.Equal(t, externalapi.EventBatchStatusAccepted, ok.Results[0].Status)

	dedupID := eventlog.DedupKey(fixtures.AcmeID, "order-1")
	assert.Equal(t, 1, env.OutboxCount(t, "event.collected", map[string]any{"backfill": true, "dedupId": dedupID}))
}

// The live path refuses history and the system's own actions; the all-or-nothing endpoint
// answers 422 with the reason, and the batch reports it per item.
func TestExternalEventsRefuseHistoryAndReservedActionsOnTheLivePath(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")
	old := externalapi.NewOptNilTimestamp(externalapi.Timestamp(eventsNow.AddDate(-2, 0, 0)))

	res, err := c.EventsCreate(context.Background(), &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{SubjectId: "s-1", Action: "order_paid", OccurredAt: old}},
	})
	require.NoError(t, err)
	refused, isRefused := res.(*externalapi.EventsCreateUnprocessableEntity)
	require.Truef(t, isRefused, "got %T", res)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, "backfill")

	batch, err := c.EventsBatchSubmit(context.Background(), &externalapi.RecordEventsBatchInput{
		Events: []externalapi.EventInput{
			{SubjectId: "s-1", Action: "order_paid", OccurredAt: old},
			{SubjectId: "s-1", Action: "email.opened"},
			{SubjectId: "s-1", Action: "order_paid"},
		},
	})
	require.NoError(t, err)
	ok, isOK := batch.(*externalapi.RecordEventsBatchResult)
	require.Truef(t, isOK, "got %T", batch)
	require.Len(t, ok.Results, 3)
	assert.Equal(t, externalapi.EventBatchStatusFailed, ok.Results[0].Status)
	stale, reserved := ok.Results[0].Error.Or(""), ok.Results[1].Error.Or("")
	assert.Contains(t, stale, "backfill")
	assert.Equal(t, externalapi.EventBatchStatusFailed, ok.Results[1].Status)
	assert.Contains(t, reserved, "reserved")
	assert.Equal(t, externalapi.EventBatchStatusAccepted, ok.Results[2].Status)
}
