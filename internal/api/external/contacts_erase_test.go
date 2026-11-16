package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// initechClient is an /api client for the Initech Workspace, the tenant that holds the
// fixture Contact carrying every dependent row (ContactErasable).
func initechClient(t *testing.T, env *testhelper.TestEnv, scopes ...apitokens.Scope) *externalapi.Client {
	t.Helper()
	return env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, scopes...))
}

func eraseErasable(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	err := initechClient(t, env, "contacts:erase").ContactsDelete(context.Background(),
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.NoError(t, err)
}

func eventByID(t *testing.T, env *testhelper.TestEnv, id int64) *ent.Event {
	t.Helper()
	e, err := env.DB.Event.Get(context.Background(), id)
	require.NoError(t, err)
	return e
}

func TestExternalContactsEraseNeedsTheEraseScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.ContactErasableID)

	err := initechClient(t, env, "contacts:read", "contacts:write").ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden, "contacts:write does not erase")
	still, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, still)

	eraseErasable(t, env)
	gone, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
}

func TestExternalContactsEraseDoesNotMoveRatesOrMetering(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := initechClient(t, env, "contacts:erase", "sending_domains:read")

	ratesOf := func() any {
		ok, err := c.SendingDomainRatesList(ctx, externalapi.SendingDomainRatesListParams{})
		require.NoError(t, err)
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
	assert.Equal(t, 2, countsBefore[systemevent.EmailSent])

	err := c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	require.NoError(t, err)

	assert.Equal(t, ratesBefore, ratesOf(), "complaint and bounce rates")
	countsAfter := countsOf()
	for _, action := range []string{
		systemevent.EmailSent, systemevent.EmailOpened, systemevent.EmailClicked, systemevent.EmailDelivered, systemevent.EmailDelayed,
		systemevent.EmailDropped, systemevent.EmailBounced, systemevent.EmailComplained, systemevent.EmailUnsubscribed,
		systemevent.ContactCreated,
	} {
		assert.Equalf(t, countsBefore[action], countsAfter[action], "%s count", action)
	}
}

func TestExternalContactsEraseIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// An Acme token cannot erase an Initech Contact: it does not exist for Acme.
	err := env.ExternalScoped(t, apitokens.ScopeContactsErase).ContactsDelete(ctx,
		externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactErasableID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	still, err := env.DB.Contact.Query().Where(contact.ID(fixtures.ContactErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, still)
	assert.Equal(t, fixtures.ContactErasableEmail, *eventByID(t, env, fixtures.EventErasableSentID).Email)
}

// An erased person who reappears is a new Contact, and the surviving opt-out still binds
// their address.
func TestExternalContactsEraseDoesNotBlockReappearance(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	eraseErasable(t, env)

	created, err := initechClient(t, env, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)
	assert.NotEqual(t, entityIDString(fixtures.ContactErasableID), created.ID)

	n, err := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ(fixtures.ContactErasableEmail), unsubscribe.WorkspaceID(fixtures.InitechID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the opt-out outlives the gap")
}

// The contact.erased signal names the API token that asked for the Erasure.
func TestExternalContactsEraseRecordsTheTokenRequester(t *testing.T) {
	env := testhelper.Setup(t)

	eraseErasable(t, env)

	msgs := env.QueuedMessages(t, systemevent.ContactErased)
	require.Len(t, msgs, 1)
	assert.Equal(t, "api_token", msgs[0].Data["requesterKind"])
	assert.NotZero(t, msgs[0].Data["requesterId"])
}
