package external_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// initechClient is an /api client for the Initech Workspace, the tenant that holds the
// fixture Contact carrying every dependent row (ContactErasable).
func initechClient(t *testing.T, env *testhelper.TestEnv, scopes ...string) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
}

func eraseErasable(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	res, err := initechClient(t, env, "contacts:erase").ContactsDelete(context.Background(),
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.NoError(t, err)
	require.IsTypef(t, &externalapi.ContactsDeleteNoContent{}, res, "got %T", res)
}

func eventByID(t *testing.T, env *testhelper.TestEnv, id int64) *ent.Event {
	t.Helper()
	e, err := env.DB.Event.Get(context.Background(), id)
	require.NoError(t, err)
	return e
}

func eventExists(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	ok, err := env.DB.Event.Query().Where(event.ID(id)).Exist(context.Background())
	require.NoError(t, err)
	return ok
}

func TestExternalContactsEraseNeedsTheEraseScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.ContactErasableID)

	denied, err := initechClient(t, env, "contacts:read", "contacts:write").ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsDeleteUnauthorized{}, denied, "contacts:write does not erase")
	still, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, still)

	eraseErasable(t, env)
	gone, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
}

func TestExternalContactsEraseRemovesThePersonsData(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	eraseErasable(t, env)

	// Removed outright: the Contact, its Visitors, Tag links, Confirmation, runs.
	n, err := env.DB.Visitor.Query().Where(visitor.ID(300)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "visitors")
	linked, err := env.DB.Tag.Query().Where(tag.ID(fixtures.TagInitechID)).QueryContacts().IDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.ContactBystanderID}, linked, "the tag stays, only the bystander keeps it")
	n, err = env.DB.Confirmation.Query().Where(confirmation.ID(fixtures.ConfirmationErasableID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "confirmation")
	n, err = env.DB.AutomationRun.Query().Where(automationrun.ID(fixtures.AutomationRunErasableID)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "automation runs")

	// Events: customer-tracked and marketing.confirmed are deleted ...
	for _, id := range []int64{
		fixtures.EventErasableCustomID, fixtures.EventErasableAnonymousID, fixtures.EventErasableConfirmedID,
	} {
		assert.Falsef(t, eventExists(t, env, id), "event %d is deleted", id)
	}

	// ... every reserved system Event stays as an anonymous row.
	for _, id := range []int64{
		fixtures.EventErasableSentID, fixtures.EventErasableOpenedID, fixtures.EventErasableClickedID,
		fixtures.EventErasableBouncedID, fixtures.EventErasableComplainedID, fixtures.EventErasableUnsubscribedID,
		fixtures.EventErasableCreatedID,
	} {
		e := eventByID(t, env, id)
		assert.Nilf(t, e.ContactID, "event %d contact_id", id)
		assert.Nilf(t, e.VisitorID, "event %d visitor_id", id)
		assert.Nilf(t, e.Email, "event %d email", id)
		assert.Nilf(t, e.Phone, "event %d phone", id)
		assert.Emptyf(t, e.SubjectID, "event %d subject_id", id)
		assert.NotNilf(t, e.OccurredAt, "event %d keeps its time", id)
	}
	// Only the non-personal delivery facts the rates are computed from survive.
	assert.Equal(t, map[string]any{"sendingDomain": "mail.initech.test"}, eventByID(t, env, fixtures.EventErasableSentID).Properties)
	assert.Equal(t, map[string]any{"sendingDomain": "mail.initech.test", "bounceKind": "permanent"}, eventByID(t, env, fixtures.EventErasableBouncedID).Properties)
	assert.Empty(t, eventByID(t, env, fixtures.EventErasableClickedID).Properties, "the clicked url is gone")
	assert.Empty(t, eventByID(t, env, fixtures.EventErasableCreatedID).Properties)

	// The refusal survives by Destination, detached from the Contact.
	unsub, err := env.DB.Unsubscribe.Get(ctx, 300)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactErasableEmail, unsub.Destination)
	assert.Nil(t, unsub.ContactID)
	supp, err := env.DB.Suppression.Get(ctx, 300)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactErasableEmail, supp.Destination)
	assert.Nil(t, supp.ContactID)

	// Delivery records stay as rows that no longer name anyone.
	for _, id := range []int64{fixtures.OutboundMessageErasableBroadcastID, fixtures.OutboundMessageErasableTransactionalID} {
		m, err := env.DB.OutboundMessage.Get(ctx, id)
		require.NoError(t, err)
		assert.Emptyf(t, m.Destination, "message %d destination", id)
		assert.Nilf(t, m.ContactID, "message %d contact_id", id)
		assert.Equal(t, outboundmessage.StatusSent, m.Status)
	}
	r, err := env.DB.BroadcastRecipient.Get(ctx, fixtures.BroadcastRecipientErasableID)
	require.NoError(t, err)
	assert.Zero(t, r.ContactID)
}

func TestExternalContactsEraseLeavesOtherContactsAlone(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	eraseErasable(t, env)

	b, err := env.DB.Contact.Get(ctx, fixtures.ContactBystanderID)
	require.NoError(t, err)
	assert.Equal(t, "ben@initech.test", *b.Email)
	for _, id := range []int64{fixtures.EventBystanderCustomID, fixtures.EventBystanderSentID} {
		e := eventByID(t, env, id)
		assert.EqualValues(t, fixtures.ContactBystanderID, *e.ContactID)
		assert.Equal(t, "ben@initech.test", *e.Email)
	}
	m, err := env.DB.OutboundMessage.Get(ctx, fixtures.OutboundMessageBystanderID)
	require.NoError(t, err)
	assert.Equal(t, "ben@initech.test", m.Destination)
	r, err := env.DB.BroadcastRecipient.Get(ctx, fixtures.BroadcastRecipientBystanderID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.ContactBystanderID, r.ContactID)
}

func TestExternalContactsEraseDoesNotMoveRatesOrMetering(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := initechClient(t, env, "contacts:erase", "sending_domains:read")

	ratesOf := func() any {
		res, err := c.SendingDomainRatesList(ctx, externalapi.SendingDomainRatesListParams{})
		require.NoError(t, err)
		ok, isOK := res.(*externalapi.SendingDomainRatesListOK)
		require.Truef(t, isOK, "got %T", res)
		return ok.Items
	}
	// The billable/reported volume: system Event rows per action in the Workspace.
	countsOf := func() map[string]int {
		var rows []struct {
			Action string `json:"action"`
			Count  int    `json:"count"`
		}
		require.NoError(t, env.DB.Event.Query().
			Where(event.WorkspaceID(fixtures.InitechID)).
			GroupBy(event.FieldAction).
			Aggregate(ent.As(ent.Count(), "count")).
			Scan(ctx, &rows))
		out := map[string]int{}
		for _, r := range rows {
			out[r.Action] = r.Count
		}
		return out
	}

	ratesBefore, countsBefore := ratesOf(), countsOf()
	assert.Equal(t, 2, countsBefore["email.sent"])

	res, err := c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.ContactsDeleteNoContent{}, res)

	assert.Equal(t, ratesBefore, ratesOf(), "complaint and bounce rates")
	countsAfter := countsOf()
	for _, action := range []string{"email.sent", "email.opened", "email.clicked", "email.bounced", "email.complained", "email.unsubscribed", "contact.created"} {
		assert.Equalf(t, countsBefore[action], countsAfter[action], "%s count", action)
	}
}

// A failure anywhere in the transaction leaves nothing partially erased.
func TestExternalContactsEraseIsAtomic(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	// Fail the very last step, deleting the Contact, after everything else was written.
	env.Bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Type() == ent.TypeContact && m.Op().Is(ent.OpDelete|ent.OpDeleteOne) {
				return nil, errors.New("injected failure")
			}
			return next.Mutate(ctx, m)
		})
	})

	_, err := initechClient(t, env, "contacts:erase").ContactsDelete(ctx,
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.Error(t, err)

	sent := eventByID(t, env, fixtures.EventErasableSentID)
	require.NotNil(t, sent.ContactID)
	assert.Equal(t, "erin@initech.test", *sent.Email)
	assert.True(t, eventExists(t, env, fixtures.EventErasableCustomID))
	assert.True(t, eventExists(t, env, fixtures.EventErasableConfirmedID))
	m, err := env.DB.OutboundMessage.Get(ctx, fixtures.OutboundMessageErasableBroadcastID)
	require.NoError(t, err)
	assert.Equal(t, "erin@initech.test", m.Destination)
	unsub, err := env.DB.Unsubscribe.Get(ctx, 300)
	require.NoError(t, err)
	assert.NotNil(t, unsub.ContactID)
	n, err := env.DB.Visitor.Query().Where(visitor.ID(300)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = env.DB.Confirmation.Query().Where(confirmation.ID(fixtures.ConfirmationErasableID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = env.DB.Suppression.Query().Where(suppression.ContactID(fixtures.ContactErasableID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = env.DB.Unsubscribe.Query().Where(unsubscribe.ContactID(fixtures.ContactErasableID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestExternalContactsEraseIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// An Acme token cannot erase an Initech Contact: it does not exist for Acme.
	res, err := env.ExternalScoped(t, "contacts:erase").ContactsDelete(ctx,
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsDeleteNotFound{}, res)

	still, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, still)
	assert.Equal(t, "erin@initech.test", *eventByID(t, env, fixtures.EventErasableSentID).Email)
}

// An erased person who reappears is a new Contact, and the surviving opt-out still binds
// their address.
func TestExternalContactsEraseDoesNotBlockReappearance(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	eraseErasable(t, env)

	res, err := initechClient(t, env, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)
	assert.NotEqual(t, entityIDString(fixtures.ContactErasableID), created.ID)

	n, err := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ(fixtures.ContactErasableEmail), unsubscribe.WorkspaceID(fixtures.InitechID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the opt-out outlives the gap")
}

// Erasure leaves one PII-free contact.erased Event (ADR 0021): the outbox message that
// projects it names the Workspace, the requester and the identifier kind, and the
// customer's subject_id rides only in that message, which the webhook delivers.
func TestExternalContactsEraseEmitsContactErasedOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	eraseErasable(t, env)

	envelopes := env.OutboxEnvelopes(t, events.NameContactErased)
	require.Len(t, envelopes, 1, "once per Erasure")
	assert.EqualValues(t, fixtures.InitechID, envelopes[0].WorkspaceID)

	var data map[string]any
	require.NoError(t, json.Unmarshal(envelopes[0].Data, &data))
	assert.Equal(t, "user-erin-300", data["subjectId"], "the customer's own id, for their downstream erasure")
	assert.Equal(t, "contact_id", data["identifierKind"])
	assert.Equal(t, "api_token", data["requesterKind"])
	assert.NotZero(t, data["requesterId"])
	assert.NotContains(t, data, "email")
	assert.NotContains(t, data, "phone")

	// The stored Event holds no PII and no Contact reference.
	require.NoError(t, events.Persist(ctx, env.DB, envelopes[0]))
	stored, err := env.DB.Event.Query().Where(event.Action(events.NameContactErased)).Only(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.InitechID, stored.WorkspaceID)
	assert.Nil(t, stored.ContactID)
	assert.Nil(t, stored.VisitorID)
	assert.Nil(t, stored.Email)
	assert.Nil(t, stored.Phone)
	assert.Empty(t, stored.SubjectID)
	assert.NotNil(t, stored.OccurredAt)
	assert.NotContains(t, stored.Properties, "subjectId")
	assert.Equal(t, "contact_id", stored.Properties["identifierKind"])
	assert.Equal(t, "api_token", stored.Properties["requesterKind"])
}

// A Contact with no subject_id is identified downstream by its sphericon id.
func TestExternalContactsEraseFallsBackToTheContactID(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := initechClient(t, env, "contacts:erase").ContactsDelete(context.Background(),
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactBystanderID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.ContactsDeleteNoContent{}, res)

	envelopes := env.OutboxEnvelopes(t, events.NameContactErased)
	require.Len(t, envelopes, 1)
	var data map[string]any
	require.NoError(t, json.Unmarshal(envelopes[0].Data, &data))
	assert.Equal(t, strconv.FormatInt(fixtures.ContactBystanderID, 10), data["subjectId"])
}

// A refused Erasure emits nothing.
func TestExternalContactsEraseRefusedEmitsNoContactErased(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.ExternalScoped(t, "contacts:erase").ContactsDelete(context.Background(),
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.ContactsDeleteNotFound{}, res)
	assert.Empty(t, env.OutboxEnvelopes(t, events.NameContactErased))
}
