package contacts_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An import that only adds new Contacts creates the new ones and leaves a Contact the Workspace has
// as it is but for the refusals its row carries, which narrow and are always recorded (ADR 0138).
func TestAnImportThatOnlyAddsNewLeavesAKnownContactButForItsRefusals(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	intake := testhelper.Module[*contacts.Module](env)
	refusal := &engagementtracking.Observation{State: engagementtracking.Denied}

	out, err := intake.Import(ctx, acme, contacts.ImportInput{
		Cause:      events.Import(42),
		OnlyAddNew: true,
		Rows: []contacts.IntakeInput{
			{
				Email:              new(fixtures.ContactAliceEmail),
				FirstName:          patch.Set("Overwritten"),
				Tags:               []string{"from-the-file"},
				EngagementTracking: engagementtracking.ContactObservations{Email: refusal},
			},
			{Email: new("only-new@example.com"), Tags: []string{"from-the-file"}},
			// A national number, read with the row's country, names the Texter.
			{Phone: new("(415) 555-0810"), Country: patch.Set("US"), FirstName: patch.Set("Overwritten")},
		},
	})
	require.NoError(t, err)
	alice, err := intake.Get(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	tracking, err := testhelper.Module[*engagementtracking.Module](env).Of(ctx, acme, channels.Email, fixtures.ContactAliceEmail)
	require.NoError(t, err)
	fresh, err := intake.Get(ctx, acme, out.Items[1].Value.Contact.ID)
	require.NoError(t, err)
	texter, err := intake.Get(ctx, acme, fixtures.ContactTexterID)
	require.NoError(t, err)

	require.NoError(t, out.Items[0].Err)
	assert.True(t, out.Items[0].Value.Kept)
	assert.False(t, out.Items[0].Value.Created)
	assert.NotEqual(t, "Overwritten", lo.FromPtr(alice.FirstName))
	assert.NotContains(t, tagNames(alice), "from-the-file")
	assert.Equal(t, engagementtracking.ReadingDenied, tracking.State, "the refusal is recorded")
	require.NoError(t, out.Items[1].Err)
	assert.True(t, out.Items[1].Value.Created)
	assert.False(t, out.Items[1].Value.Kept)
	assert.Contains(t, tagNames(fresh), "from-the-file")
	require.NoError(t, out.Items[2].Err)
	assert.True(t, out.Items[2].Value.Kept)
	assert.Equal(t, "Tess", lo.FromPtr(texter.FirstName))
}

// What an import's rows publish carries the job's cause, as history, a Contact it creates has the
// source `import`, and the rows record no Audit entry: the job records one for the whole import.
func TestAnImportMarksWhatItsRowsPublishAndAuditsNothingItself(t *testing.T) {
	env := testhelper.Setup(t)
	intake := testhelper.Module[*contacts.Module](env)

	out, err := intake.Import(t.Context(), env.DB.Scoped(fixtures.AcmeID), contacts.ImportInput{
		Cause: events.Import(42),
		Rows:  []contacts.IntakeInput{{Email: new("caused@example.com")}},
	})
	require.NoError(t, err)
	live := env.QueuedEnvelopes(t, systemevent.ContactCreated)
	created := lo.Filter(env.StoredEvents(t, systemevent.ContactCreated), func(e *ent.Event, _ int) bool {
		return lo.FromPtr(e.ContactID) == out.Items[0].Value.Contact.ID
	})
	audited := testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry)

	require.NoError(t, out.Items[0].Err)
	require.Len(t, created, 1)
	assert.True(t, events.WasHistory(created[0]), "history")
	assert.Empty(t, live, "history is queued for no live consumer")
	assert.Equal(t, contact.SourceImport, out.Items[0].Value.Contact.Source)
	assert.Empty(t, audited)
}
