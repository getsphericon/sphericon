package external_test

import (
	"context"
	"sort"
	"testing"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectedRow is an outbox row decoded back into its CollectedEvent.
type collectedRow struct {
	envelope events.Envelope
	event    *events.CollectedEvent
}

// outboxCollected reads every domain-event outbox row and decodes it as a
// CollectedEvent. External ingest now publishes through the bus (the persist
// subscriber writes the Event row asynchronously; the router isn't run under
// txdb), so the outbox is where the assertions land.
func outboxCollected(t *testing.T, env *testhelper.TestEnv) []collectedRow {
	t.Helper()
	var out []collectedRow
	for _, envlp := range env.OutboxEnvelopes(t) {
		require.Equal(t, events.NameCollected, envlp.Name)
		decoded, err := events.Decode(envlp)
		require.NoError(t, err)
		ce, ok := decoded.(*events.CollectedEvent)
		require.Truef(t, ok, "got %T", decoded)
		out = append(out, collectedRow{envelope: envlp, event: ce})
	}
	return out
}

func TestExternalContactsCRUD(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "contacts:read", "contacts:write", "contacts:erase")
	ctx := context.Background()

	list, err := c.ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	listed, ok := list.(*externalapi.ContactsListOK)
	require.Truef(t, ok, "got %T", list)
	assert.NotEmpty(t, listed.Items, "the workspace returns its contacts")

	created, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email:     externalapi.NewOptNilEmailAddress("new@example.com"),
		FirstName: externalapi.NewOptNilString("New"),
	})
	require.NoError(t, err)
	contact, ok := created.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", created)
	email, ok := contact.Email.Get()
	require.True(t, ok)
	assert.Equal(t, externalapi.EmailAddress("new@example.com"), email)

	got, err := c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactResource{}, got)

	missing, err := c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsGetNotFound{}, missing)

	deleted, err := c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: entityIDString(fixtures.ContactAliceID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsDeleteNoContent{}, deleted)
}

// A contact created through /api enrolls into Automations like a UI-created one:
// it publishes contact.created onto the outbox in the same transaction.
func TestExternalContactsCreatePublishesContactCreated(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress("published@example.com"),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)

	n := env.OutboxCount(t, "contact.created", map[string]any{"email": "published@example.com", "contactId": created.ID})
	assert.Equal(t, 1, n, "contact.created published once")
}

// Isolated: the unique violation aborts this test's transaction.
func TestExternalContactsConflict(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsCreateConflict{}, res)
}

func TestExternalAuthAndScopes(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	ctx := context.Background()

	// No token / bad token → 401 Unauthorized variant.
	res, err := env.ExternalAnonymous(t).ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListUnauthorized{}, res)

	res, err = env.ExternalWithToken(t, "omtk_deadbeef_invalidsecret").ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListUnauthorized{}, res)

	// Read-only token: can read, cannot create.
	ro := env.ExternalScoped(t, "contacts:read")
	createRes, err := ro.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("x@example.com")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsCreateUnauthorized{}, createRes)

	listRes, err := ro.ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListOK{}, listRes)
}

// TestExternalEventsCreate verifies a batch ingest persists events scoped to the
// token's workspace with all optional fields mapped — querying the DB afterwards
// is what catches the NOT-NULL workspace_id bug (a bare 204 assertion would not).
func TestExternalEventsCreate(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")
	ctx := context.Background()

	occurred := eventsNow.Add(-time.Hour) // inside the live window
	res, err := c.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{
			{
				SubjectId:  "user:dave@example.com",
				Action:     "signup",
				Email:      externalapi.NewOptNilEmailAddress("dave@example.com"),
				OccurredAt: externalapi.NewOptNilTimestamp(externalapi.Timestamp(occurred)),
				Properties: externalapi.NewOptNilEventInputProperties(externalapi.EventInputProperties{
					"plan":   jx.Raw(`"pro"`),
					"amount": jx.Raw(`42`),
				}),
			},
			{SubjectId: "user:erin@example.com", Action: "login"},
		},
	})
	require.NoError(t, err)
	require.IsType(t, &externalapi.EventsCreateNoContent{}, res)

	// Both events are published to the outbox as CollectedEvents, scoped to the
	// token's workspace, with every optional field carried as-is.
	byID := map[string]collectedRow{}
	rows := outboxCollected(t, env)
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
	ro := env.ExternalScoped(t, "events:read")
	createRes, err := ro.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{SubjectId: "s", Action: "a"}},
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EventsCreateUnauthorized{}, createRes)

	// events:write cannot read actions.
	wo := env.ExternalScoped(t, "events:write")
	listRes, err := wo.EventActionsList(ctx, externalapi.EventActionsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EventActionsListUnauthorized{}, listRes)
}

// TestExternalEventActionsList verifies distinct actions for the token's
// workspace are returned, sorted, with correct pagination totals. The two
// fixture events (page_view, purchase) belong to workspace 1.
func TestExternalEventActionsList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "events:read")

	res, err := c.EventActionsList(context.Background(), externalapi.EventActionsListParams{})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.EventActionsListOK)
	require.Truef(t, isOK, "got %T", res)

	// Distinct actions for the token's workspace, sorted. Selected by membership so
	// the assertion is independent of how many actions are seeded.
	actions := make([]string, len(ok.Items))
	for i, item := range ok.Items {
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
	c := env.ExternalScoped(t, "events:read", "events:write")

	// Listing for workspace 1 must not surface workspace 2's action.
	res, err := c.EventActionsList(ctx, externalapi.EventActionsListParams{})
	require.NoError(t, err)
	ok := res.(*externalapi.EventActionsListOK)
	acmeActions := make([]string, len(ok.Items))
	for i, item := range ok.Items {
		assert.NotEqual(t, fixtures.EventGlobexAction, item.Action, "another tenant's action leaked")
		acmeActions[i] = item.Action
	}
	assert.Contains(t, acmeActions, "page_view", "the workspace's own actions are returned")

	// Writes land in workspace 1, not the token-agnostic global pool.
	_, err = c.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{SubjectId: "user:fred@example.com", Action: "ws1_write"}},
	})
	require.NoError(t, err)
	rows := outboxCollected(t, env)
	require.Len(t, rows, 1)
	assert.Equal(t, "user:fred@example.com", rows[0].event.SubjectID)
	assert.EqualValues(t, fixtures.AcmeID, rows[0].envelope.WorkspaceID)
}

func TestExternalRequestValidation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	// An invalid email is refused by ogen's validation with the documented 400.
	res, err := c.ContactsCreate(context.Background(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("not-an-email")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsCreateBadRequest{}, res)
}
