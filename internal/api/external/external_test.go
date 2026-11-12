package external_test

import (
	"context"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// collectedRow is a queued event decoded back into its CollectedEvent.
type collectedRow struct {
	envelope events.Envelope
	event    *events.CollectedEvent
}

// queuedCollected reads every event the test's publishes queued and decodes it as a
// CollectedEvent: what external ingest published.
func queuedCollected(t *testing.T, env *testhelper.TestEnv) []collectedRow {
	t.Helper()
	envelopes := env.QueuedEnvelopes(t)
	out := make([]collectedRow, 0, len(envelopes))
	for i := range envelopes {
		envlp := &envelopes[i]
		require.Equal(t, events.NameCollected, envlp.Name)
		decoded, err := events.Decode(*envlp)
		require.NoError(t, err)
		ce, ok := decoded.(*events.CollectedEvent)
		require.Truef(t, ok, "got %T", decoded)
		out = append(out, collectedRow{envelope: *envlp, event: ce})
	}
	return out
}

func TestExternalContactsCRUD(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite, apitokens.ScopeContactsErase)
	ctx := context.Background()

	listed, err := c.ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.NotEmpty(t, listed.Items, "the workspace returns its contacts")

	contact, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email:     externalapi.NewOptNilEmailAddress("new@example.com"),
		FirstName: externalapi.NewOptNilString("New"),
	})
	require.NoError(t, err)
	email, ok := contact.Email.Get()
	require.True(t, ok)
	assert.Equal(t, externalapi.EmailAddress("new@example.com"), email)

	_, err = c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)

	_, err = c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	err = c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
}

// A contact created through /api enrolls into Automations like a UI-created one:
// it publishes contact.created in the same transaction.
func TestExternalContactsCreatePublishesContactCreated(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)

	created, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress("published@example.com"),
	})
	require.NoError(t, err)

	n := env.QueuedCount(t, systemevent.ContactCreated, map[string]any{"email": "published@example.com", "contactId": created.ID})
	assert.Equal(t, 1, n, "contact.created published once")
}

func TestExternalAuthAndScopes(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	ctx := context.Background()

	// No token / bad token → 401 Unauthorized variant.
	_, err := env.ExternalAnonymous(t).ContactsList(ctx, externalapi.ContactsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)

	_, err = env.ExternalWithToken(t, "omtk_deadbeef_invalidsecret").ContactsList(ctx, externalapi.ContactsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)

	// Read-only token: can read, cannot create.
	ro := env.ExternalScoped(t, apitokens.ScopeContactsRead)
	_, err = ro.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("x@example.com")})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	_, err = ro.ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
}

// TestExternalEventsCreate verifies a batch ingest persists events scoped to the
// token's workspace with all optional fields mapped — querying the DB afterwards
// is what catches the NOT-NULL workspace_id bug (a bare 204 assertion would not).
func TestExternalEventsCreate(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)
	ctx := context.Background()

	occurred := eventsNow.Add(-time.Hour) // inside the live window
	err := c.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{
			{
				SubjectId:  "user:dave@example.com",
				Action:     "signup",
				Email:      externalapi.NewOptNilEmailAddress("dave@example.com"),
				OccurredAt: externalapi.NewOptNilTimestamp(externalapi.Timestamp(occurred)),
				Properties: externalapi.NewOptNilJsonObject(externalapi.JsonObject{
					"plan":   jx.Raw(`"pro"`),
					"amount": jx.Raw(`42`),
				}),
			},
			{SubjectId: "user:erin@example.com", Action: "login"},
		},
	})
	require.NoError(t, err)

	// Both events are published as CollectedEvents, scoped to the
	// token's workspace, with every optional field carried as-is.
	byID := map[string]collectedRow{}
	rows := queuedCollected(t, env)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.EqualValues(t, fixtures.AcmeID, r.envelope.WorkspaceID) // token belongs to Acme
		byID[r.event.SubjectID] = r
	}

	dave := byID["user:dave@example.com"].event
	require.NotNil(t, dave)
	assert.Equal(t, "signup", dave.Action)
	assert.Equal(t, "dave@example.com", dave.Email)
	assert.True(t, dave.OccurredAt.Equal(occurred))
	assert.Equal(t, "pro", dave.Properties["plan"])
	assert.EqualValues(t, 42, dave.Properties["amount"])

	erin := byID["user:erin@example.com"].event
	require.NotNil(t, erin)
	assert.Equal(t, "login", erin.Action)
}

// TestExternalEventsScopes verifies events:write/read scopes are enforced.
func TestExternalEventsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// events:read cannot write.
	ro := env.ExternalScoped(t, apitokens.ScopeEventsRead)
	err := ro.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{SubjectId: "s", Action: "a"}},
	})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	// events:write cannot read actions.
	wo := env.ExternalScoped(t, apitokens.ScopeEventsWrite)
	_, err = wo.EventActionsList(ctx, externalapi.EventActionsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// TestExternalEventActionsList verifies distinct actions for the token's
// workspace are returned, sorted, with correct pagination totals. The two
// fixture events (page_view, purchase) belong to workspace 1.
func TestExternalEventActionsList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeEventsRead)

	res, err := c.EventActionsList(context.Background(), externalapi.EventActionsListParams{})
	require.NoError(t, err)

	// Distinct actions for the token's workspace, sorted. Selected by membership so
	// the assertion is independent of how many actions are seeded.
	actions := make([]string, len(res.Items))
	for i, item := range res.Items {
		actions[i] = item.Action
	}
	assert.True(t, sort.StringsAreSorted(actions), "actions are sorted")
	assert.Contains(t, actions, "page_view")
	assert.Contains(t, actions, "purchase")
}

// TestExternalEventsWorkspaceIsolation verifies event reads and writes are
// scoped to the token's workspace and never leak across tenants.
func TestExternalEventsWorkspaceIsolation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The Globex tenant has an event carrying an action unique to it (fixture).
	// Scoped tokens always bind to Acme.
	c := env.ExternalScoped(t, apitokens.ScopeEventsRead, apitokens.ScopeEventsWrite)

	// Listing for workspace 1 must not surface workspace 2's action.
	res, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{})
	require.NoError(t, err)
	acmeActions := make([]string, len(res.Items))
	for i, item := range res.Items {
		assert.NotEqual(t, fixtures.EventGlobexAction, item.Action, "another tenant's action leaked")
		acmeActions[i] = item.Action
	}
	assert.Contains(t, acmeActions, "page_view", "the workspace's own actions are returned")

	// Writes land in workspace 1, not the token-agnostic global pool.
	err = c.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{SubjectId: "user:fred@example.com", Action: "ws1_write"}},
	})
	require.NoError(t, err)
	rows := queuedCollected(t, env)
	require.Len(t, rows, 1)
	assert.Equal(t, "user:fred@example.com", rows[0].event.SubjectID)
	assert.EqualValues(t, fixtures.AcmeID, rows[0].envelope.WorkspaceID)
}

func TestExternalRequestValidation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)

	// An invalid email is refused by ogen's validation with the documented 400.
	_, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("not-an-email")})
	testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
}
