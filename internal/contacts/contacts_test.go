package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures: workspace Acme owns contacts Alice and Bob. No other workspace owns them.

func createdEvents(t *testing.T, env *testhelper.TestEnv, contactID int64) int {
	t.Helper()
	return env.OutboxCount(t, "contact.created", map[string]any{"contactId": contactID})
}

func TestCreatePersistsAndPublishesContactCreated(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	c, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{
		Email:        new("  New.Person@Example.com "),
		FirstName:    new("New"),
		CustomFields: map[string]any{"plan": "pro"},
	})
	require.NoError(t, err)

	assert.Equal(t, "new.person@example.com", lo.FromPtr(c.Email), "email alias key is normalized")
	assert.Equal(t, "New", lo.FromPtr(c.FirstName))
	assert.Equal(t, "pro", c.CustomFields["plan"])
	assert.Equal(t, 1, createdEvents(t, env, c.ID), "contact.created published once, in the same transaction")
}

func TestCreateConflictOnAliasKey(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	// Case and whitespace do not make a different email.
	_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{Email: new(" ALICE@example.com ")})
	require.ErrorIs(t, err, contacts.ErrEmailTaken)
	assert.Equal(t, contacts.FieldEmail, contacts.ErrEmailTaken.Field())
	assert.Equal(t, "email already exists", contacts.ErrEmailTaken.Message())
}

func TestCreateConflictOnPhone(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{Phone: new("+15550001")})
	require.NoError(t, err)

	_, err = m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{Phone: new("+15550001")})
	require.ErrorIs(t, err, contacts.ErrPhoneTaken)
}

func TestCreateConflictOnSubjectID(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{SubjectID: new("user-new-1")})
	require.NoError(t, err)

	_, err = m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{SubjectID: new("user-new-1")})
	require.ErrorIs(t, err, contacts.ErrSubjectIDTaken)
}

func TestUpdateChangesOnlyGivenAttributes(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	c, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, contacts.UpdateInput{LastName: patch.Set("Jones")})
	require.NoError(t, err)
	assert.Equal(t, "Jones", lo.FromPtr(c.LastName))
	assert.Equal(t, "Alice", lo.FromPtr(c.FirstName), "untouched attribute stays")
	assert.Equal(t, "alice@example.com", lo.FromPtr(c.Email))
	assert.Zero(t, createdEvents(t, env, fixtures.ContactAliceID), "an update is not a creation")
}

func TestUpdateNotFoundAcrossWorkspaces(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), 999999, contacts.UpdateInput{})
	require.ErrorIs(t, err, contacts.ErrNotFound)

	_, err = m.Update(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceID, contacts.UpdateInput{LastName: patch.Set("X")})
	assert.ErrorIs(t, err, contacts.ErrNotFound, "another workspace cannot touch the contact")
}

func TestUpdateConflict(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, contacts.UpdateInput{Email: patch.Set("bob@example.com")})
	require.ErrorIs(t, err, contacts.ErrEmailTaken)
}

func TestUpsertResolvesExistingContactByAliasKey(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	res, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.UpsertInput{
		Email:        new("Alice@example.com"),
		Phone:        new("+15550002"),
		FirstName:    patch.Set("Alicia"),
		CustomFields: map[string]any{"plan": "team"},
	})
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.Equal(t, int64(fixtures.ContactAliceID), res.Contact.ID)
	assert.Equal(t, "+15550002", lo.FromPtr(res.Contact.Phone), "a missing alias key is filled in")
	assert.Equal(t, "team", res.Contact.CustomFields["plan"])
	assert.Equal(t, "Alicia", lo.FromPtr(res.Contact.FirstName), "an asserted attribute overwrites the stored one")
	assert.Zero(t, createdEvents(t, env, fixtures.ContactAliceID))

	// The newly added phone now resolves the same contact.
	again, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.UpsertInput{Phone: new("+15550002")})
	require.NoError(t, err)
	assert.Equal(t, int64(fixtures.ContactAliceID), again.Contact.ID)
}

func TestUpsertCreatesAndPublishesOnce(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	res, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.UpsertInput{SubjectID: new("user-77"), Email: new("u77@example.com")})
	require.NoError(t, err)
	assert.True(t, res.Created)
	assert.Equal(t, 1, createdEvents(t, env, res.Contact.ID))

	again, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.UpsertInput{SubjectID: new("user-77")})
	require.NoError(t, err)
	assert.False(t, again.Created)
	assert.Equal(t, res.Contact.ID, again.Contact.ID)
	assert.Equal(t, 1, createdEvents(t, env, res.Contact.ID), "still published once")
}

func TestUpsertRequiresAnAliasKey(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	_, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.UpsertInput{FirstName: patch.Set("Nobody"), Email: new("  ")})
	assert.ErrorIs(t, err, contacts.ErrIdentityRequired)
}

func TestUpsertDoesNotResolveAnotherWorkspacesContact(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	res, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.GlobexID), contact.SourceManual, contacts.UpsertInput{Email: lo.ToPtr(fixtures.ContactAliceEmail)})
	require.NoError(t, err)
	assert.True(t, res.Created, "Alice's email is free in Globex, so a new contact is made")
	assert.NotEqual(t, int64(fixtures.ContactAliceID), res.Contact.ID)
	assert.Equal(t, int64(fixtures.GlobexID), res.Contact.WorkspaceID)

	found, err := contacts.Resolve(context.Background(), env.DB.Scoped(fixtures.GlobexID), nil, lo.ToPtr(fixtures.ContactBobEmail), nil)
	require.NoError(t, err)
	assert.Nil(t, found, "an Acme contact is invisible to a Globex-scoped resolve")
}

func TestResolveIDFindsExistingContactsOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	id, err := contacts.ResolveID(ctx, env.DB.Scoped(fixtures.AcmeID), "", lo.ToPtr(fixtures.ContactAliceEmail), nil)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.ContactAliceID, id)

	id, err = contacts.ResolveID(ctx, env.DB.Scoped(fixtures.AcmeID), "", new("nobody@example.com"), nil)
	require.NoError(t, err)
	assert.Zero(t, id, "never creates")

	id, err = contacts.ResolveID(ctx, env.DB.Scoped(fixtures.GlobexID), "", lo.ToPtr(fixtures.ContactAliceEmail), nil)
	require.NoError(t, err)
	assert.Zero(t, id, "another workspace's contact is not resolved")
}
