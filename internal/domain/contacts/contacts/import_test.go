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

// A row whose keys name two Contacts is written to the one the keys' rank picks, without the keys
// of the other, and warned (ADR 0138): the email names Alice and the phone the Texter, so Alice is
// written and keeps no phone, and the Texter is left as it was.
func TestAnImportRowWhoseKeysNameTwoContactsIsWrittenByTheKeysRank(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	intake := testhelper.Module[*contacts.Module](env)

	out, err := intake.Import(ctx, acme, contacts.ImportInput{
		Cause: events.Import(42),
		Rows: []contacts.IntakeInput{{
			Email: new(fixtures.ContactAliceEmail), Phone: new(fixtures.ContactTexterPhone), City: patch.Set("Lisbon"),
		}},
	})
	require.NoError(t, err)
	alice, err := intake.Get(ctx, acme, fixtures.ContactAliceID)
	require.NoError(t, err)
	texter, err := intake.Get(ctx, acme, fixtures.ContactTexterID)
	require.NoError(t, err)

	require.NoError(t, out.Items[0].Err)
	assert.EqualValues(t, fixtures.ContactAliceID, out.Items[0].Value.Contact.ID)
	assert.Contains(t, out.Items[0].Value.Warnings, contacts.WarningKeysNameTwoContacts)
	assert.Equal(t, "Lisbon", lo.FromPtr(alice.City))
	assert.Nil(t, alice.Phone)
	assert.Equal(t, fixtures.ContactTexterPhone, lo.FromPtr(texter.Phone))
	assert.Nil(t, texter.City)
}

// The API's batch upsert refuses the same row as a key taken: only a CSV import ranks the keys.
func TestABatchUpsertRefusesARowWhoseKeysNameTwoContacts(t *testing.T) {
	env := testhelper.Setup(t)
	intake := testhelper.Module[*contacts.Module](env)

	out, err := intake.UpsertBatch(t.Context(), env.DB.Scoped(fixtures.AcmeID), contact.SourceAPI, []contacts.IntakeInput{{
		Email: new(fixtures.ContactAliceEmail), Phone: new(fixtures.ContactTexterPhone),
	}})
	require.NoError(t, err)

	require.ErrorIs(t, out.Items[0].Err, contacts.ErrPhoneTaken)
}

// The intake's verdicts on rows it is not given judge them as it would, writing nothing: a row
// without a key and an address that is none are refused, a role's address is warned.
func TestTheIntakeJudgesRowsWithoutWritingThem(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	intake := testhelper.Module[*contacts.Module](env)

	verdicts, err := intake.Verdicts(t.Context(), acme, []contacts.IntakeInput{
		{FirstName: patch.Set("Nobody")},
		{Email: new("not an address")},
		{Email: new("info@example.com")},
	})
	require.NoError(t, err)
	unwritten, err := intake.Resolve(t.Context(), acme, contacts.AliasKeys{Email: new("info@example.com")})
	require.NoError(t, err)

	require.Len(t, verdicts.Items, 3)
	require.ErrorIs(t, verdicts.Items[0].Err, contacts.ErrIdentityRequired)
	require.Error(t, verdicts.Items[1].Err)
	require.NoError(t, verdicts.Items[2].Err)
	assert.Equal(t, []contacts.Warning{contacts.WarningRoleEmail}, verdicts.Items[2].Value)
	assert.Nil(t, unwritten)
}
