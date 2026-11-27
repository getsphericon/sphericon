package entclient_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every update of an edited row that changes something moves its revision (ADR 0119, ADR 0168): no
// column is exempt, whoever writes it, so the delivery worker's turn-off of an endpoint makes an
// open editor stale as an owner's edit does.
func TestEveryUpdateThatChangesSomethingMovesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := int64(fixtures.WebhookCodebasicsID)
	revision := func() int64 {
		t.Helper()
		return env.DB.WebhookEndpoint.GetX(ctx, id).Revision
	}
	before := revision()

	env.DB.WebhookEndpoint.UpdateOneID(id).SetURL("https://hooks.example.com/renamed").ExecX(ctx)
	assert.Equal(t, before+1, revision(), "a column an editor sends")

	env.DB.WebhookEndpoint.UpdateOneID(id).AddHealthGeneration(1).ExecX(ctx)
	assert.Equal(t, before+2, revision(), "a column no editor sends")

	env.DB.WebhookEndpoint.UpdateOneID(id).SetDisabledAt(time.Now()).SetDisabledReason(webhookendpoint.DisabledReasonGone).ExecX(ctx)
	assert.Equal(t, before+3, revision(), "a job's write")
}

// An update that writes no column but updated_at (a Contact's Tag change touches the Contact) is a
// write of what an editor shows: it moves the revision.
func TestAnUpdateOfNoColumnMovesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID).Revision

	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).ExecX(ctx)

	assert.Equal(t, before+1, env.DB.Contact.GetX(ctx, fixtures.ContactAliceID).Revision)
}

// A write that changes nothing writes nothing (ADR 0119): an update that sets every column it names
// to the value the row holds moves neither the revision nor updated_at, so a save that changed
// nothing leaves every open editor fresh.
func TestAnUpdateThatChangesNothingWritesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	got := env.DB.EmailTemplate.UpdateOneID(before.ID).SetName(before.Name).SetSubject(before.Subject).SaveX(ctx)

	after := env.DB.EmailTemplate.GetX(ctx, before.ID)
	assert.Equal(t, before.Revision, after.Revision)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at stays")
	assert.Equal(t, before.Revision, got.Revision, "the update returns the row as it is")
}

// What a write that changes nothing is: a JSON value compares by value, whatever the order of its
// keys, and a time at the microseconds Postgres keeps; an edge change, a sealed column (sealed anew,
// the same plaintext reads as new ciphertext) and a no-op over a stale revision are no no-ops.
func TestWhatAnUnchangedWriteIs(t *testing.T) {
	moment := time.Date(2026, 3, 1, 12, 0, 0, 123456789, time.UTC)
	cases := []struct {
		name      string
		prepare   func(ctx context.Context, env *testhelper.TestEnv) // the stored state the write compares with
		write     func(ctx context.Context, env *testhelper.TestEnv) error
		unchanged bool
	}{
		{
			name: "JSON with its keys reordered",
			prepare: func(ctx context.Context, env *testhelper.TestEnv) {
				env.DB.EmailTemplate.UpdateOneID(fixtures.TemplateWelcomeID).SetDoc(json.RawMessage(`{"a":1,"b":{"c":2,"d":3}}`)).ExecX(ctx)
			},
			write: func(ctx context.Context, env *testhelper.TestEnv) error {
				return env.DB.EmailTemplate.UpdateOneID(fixtures.TemplateWelcomeID).SetDoc(json.RawMessage(`{"b":{"d":3,"c":2},"a":1}`)).Exec(ctx)
			},
			unchanged: true,
		},
		{
			name: "JSON of another value",
			prepare: func(ctx context.Context, env *testhelper.TestEnv) {
				env.DB.EmailTemplate.UpdateOneID(fixtures.TemplateWelcomeID).SetDoc(json.RawMessage(`{"a":1}`)).ExecX(ctx)
			},
			write: func(ctx context.Context, env *testhelper.TestEnv) error {
				return env.DB.EmailTemplate.UpdateOneID(fixtures.TemplateWelcomeID).SetDoc(json.RawMessage(`{"a":2}`)).Exec(ctx)
			},
		},
		{
			name: "a time with nanoseconds",
			prepare: func(ctx context.Context, env *testhelper.TestEnv) {
				env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSecondFactorRequiredAt(moment).ExecX(ctx)
			},
			write: func(ctx context.Context, env *testhelper.TestEnv) error {
				return env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSecondFactorRequiredAt(moment.Add(100 * time.Nanosecond)).Exec(ctx)
			},
			unchanged: true,
		},
		{
			name: "a time a microsecond later",
			prepare: func(ctx context.Context, env *testhelper.TestEnv) {
				env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSecondFactorRequiredAt(moment).ExecX(ctx)
			},
			write: func(ctx context.Context, env *testhelper.TestEnv) error {
				return env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSecondFactorRequiredAt(moment.Add(time.Microsecond)).Exec(ctx)
			},
		},
		{
			name: "a sealed column sealed anew",
			prepare: func(ctx context.Context, env *testhelper.TestEnv) {
				env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetConfigEncrypted(seal(t, `{"key":"same"}`)).ExecX(ctx)
			},
			write: func(ctx context.Context, env *testhelper.TestEnv) error {
				return env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetConfigEncrypted(seal(t, `{"key":"same"}`)).Exec(ctx)
			},
		},
		{
			name:    "an edge change, even one that moves no row",
			prepare: func(context.Context, *testhelper.TestEnv) {},
			write: func(ctx context.Context, env *testhelper.TestEnv) error {
				return env.DB.Workspace.UpdateOneID(fixtures.AcmeID).RemoveTagIDs(fixtures.TagGlobexID).Exec(ctx)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			tc.prepare(ctx, env)
			before := revisionOf(ctx, env)

			err := tc.write(ctx, env)
			after := revisionOf(ctx, env)

			require.NoError(t, err)
			assert.Equal(t, tc.unchanged, before == after, "the write leaves the revision and updated_at: %v", tc.unchanged)
		})
	}
}

// A write that changes nothing over a revision the row has moved past is refused like any other.
func TestANoOpOverAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	err := env.DB.EmailTemplate.UpdateOneID(before.ID).Where(emailtemplate.Revision(before.Revision - 1)).SetName(before.Name).Exec(ctx)

	assert.True(t, ent.IsNotFound(err), "%v", err)
}

// stamp is what moves on a write of the rows the cases touch: the revision and updated_at of each.
type stamp struct {
	template, workspace, integration       int64
	templateAt, workspaceAt, integrationAt time.Time
}

func revisionOf(ctx context.Context, env *testhelper.TestEnv) stamp {
	tpl := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)
	ws := env.DB.Workspace.GetX(ctx, fixtures.AcmeID)
	in := env.DB.Integration.GetX(ctx, fixtures.IntegrationAcmeDefaultID)
	return stamp{tpl.Revision, ws.Revision, in.Revision, tpl.UpdatedAt.UTC(), ws.UpdatedAt.UTC(), in.UpdatedAt.UTC()}
}

func seal(t *testing.T, plaintext string) string {
	t.Helper()
	sealed, err := testhelper.Cipher(t).Encrypt([]byte(plaintext))
	require.NoError(t, err)
	return sealed
}
