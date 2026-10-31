package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func fieldType(t *testing.T, env *testhelper.TestEnv, key string) customfield.Type {
	t.Helper()
	f, err := env.DB.CustomField.Query().
		Where(customfield.WorkspaceID(fixtures.AcmeID), customfield.Key(key)).Only(context.Background())
	require.NoError(t, err)
	return f.Type
}

func TestUpdateReplacesCustomFieldsAndDeclaresThemTyped(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()

	c, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, contacts.UpdateInput{
		CustomFields: patch.Set(map[string]any{"vip": true, "visits": float64(3), "plan": "pro", "": "ignored"}),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"vip": true, "visits": float64(3), "plan": "pro"}, c.CustomFields, "the blank key is dropped")
	assert.Equal(t, customfield.TypeBool, fieldType(t, env, "vip"))
	assert.Equal(t, customfield.TypeNumber, fieldType(t, env, "visits"))
	assert.Equal(t, customfield.TypeString, fieldType(t, env, "plan"))

	replaced, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, contacts.UpdateInput{
		CustomFields: patch.Set(map[string]any{"plan": "free"}),
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"plan": "free"}, replaced.CustomFields, "the given set replaces the stored one")
}

func TestUpdateClearsExplicitlyClearedAttributes(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()

	full, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{
		SubjectID: new("user-1"), Email: new("clear.me@example.com"), Phone: new("+15559990"),
		FirstName: new("Clear"), LastName: new("Me"), TimeZone: new("Europe/Berlin"),
		CustomFields: map[string]any{"plan": "pro"},
	})
	require.NoError(t, err)

	cleared, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), full.ID, contacts.UpdateInput{
		SubjectID: patch.Clear[string](), Email: patch.Clear[string](), Phone: patch.Clear[string](),
		FirstName: patch.Clear[string](), LastName: patch.Clear[string](), TimeZone: patch.Clear[string](),
		CustomFields: patch.Clear[map[string]any](),
	})
	require.NoError(t, err)
	assert.Nil(t, cleared.SubjectID)
	assert.Nil(t, cleared.Email)
	assert.Nil(t, cleared.Phone)
	assert.Nil(t, cleared.FirstName)
	assert.Nil(t, cleared.LastName)
	assert.Nil(t, cleared.TimeZone)
	assert.Empty(t, cleared.CustomFields)

	stored, err := env.DB.Contact.Get(ctx, full.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.Email, "clearing is persisted")
}

func TestCreateAndUpdateSetTheTagsAndReturnTheContactWithThem(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	created, err := m.Create(ctx, s, contact.SourceManual, contacts.CreateInput{
		Email: new("tagged@example.com"), Tags: []string{"beta", "alpha"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, tagNames(created), "Create returns the Contact with its tags")

	renamed, err := m.Update(ctx, s, created.ID, contacts.UpdateInput{FirstName: patch.Set("Tag")})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, tagNames(renamed), "absent tags keep the tag set")

	retagged, err := m.Update(ctx, s, created.ID, contacts.UpdateInput{Tags: patch.Set([]string{"gamma"})})
	require.NoError(t, err)
	assert.Equal(t, []string{"gamma"}, tagNames(retagged), "set tags become the complete tag set")
	assert.Equal(t, "Tag", lo.FromPtr(retagged.FirstName))
}

func TestUpdateKeepsAnAliasKeySetBlank(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	c, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID,
		contacts.UpdateInput{Email: patch.Set("  ")})
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, lo.FromPtr(c.Email))
}

func tagNames(c *ent.Contact) []string {
	return lo.Map(c.Edges.Tags, func(t *ent.Tag, _ int) string { return t.Name })
}

func TestUpsertBatchRunsEachItemIndependentlyAndInOrder(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	out := m.UpsertBatch(context.Background(), env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, []contacts.Attributes{
		{Email: new("batch.new@example.com"), CustomFields: map[string]any{"source": "import"}},
		{Email: lo.ToPtr(fixtures.ContactAliceEmail), FirstName: new("Ignored")},
		{FirstName: new("No identity")},
	})
	require.Len(t, out, 3)

	require.NoError(t, out[0].Err)
	assert.True(t, out[0].Result.Created)
	assert.Equal(t, "import", out[0].Result.Contact.CustomFields["source"])
	assert.Equal(t, 1, createdEvents(t, env, out[0].Result.Contact.ID))

	require.NoError(t, out[1].Err)
	assert.False(t, out[1].Result.Created)
	assert.EqualValues(t, fixtures.ContactAliceID, out[1].Result.Contact.ID)
	assert.Equal(t, "Alice", lo.FromPtr(out[1].Result.Contact.FirstName), "an existing attribute is never overwritten")

	require.ErrorIs(t, out[2].Err, contacts.ErrIdentityRequired, "a failing item does not stop the batch")
}

func TestUpsertEnrichesAnExistingContactFoundByAnotherKey(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()

	bare, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{Phone: new("+15557770")})
	require.NoError(t, err)

	res, err := m.Upsert(ctx, env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.Attributes{
		Phone: new("+15557770"), Email: new("Filled.In@Example.com"), SubjectID: new("sub-9"),
		CustomFields: map[string]any{"tier": "gold"},
	})
	require.NoError(t, err)
	assert.False(t, res.Created)
	assert.Equal(t, bare.ID, res.Contact.ID)
	assert.Equal(t, "filled.in@example.com", lo.FromPtr(res.Contact.Email), "missing keys are filled")
	assert.Equal(t, "sub-9", lo.FromPtr(res.Contact.SubjectID))
	assert.Equal(t, "gold", res.Contact.CustomFields["tier"])

	again, err := m.Upsert(ctx, env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.Attributes{Phone: new("+15557770"), CustomFields: map[string]any{"visits": float64(1)}})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"tier": "gold", "visits": float64(1)}, again.Contact.CustomFields, "custom fields merge")
}

func TestUpsertIsScopedToTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	res, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.GlobexID), contact.SourceManual, contacts.Attributes{Email: lo.ToPtr(fixtures.ContactAliceEmail)})
	require.NoError(t, err)
	assert.True(t, res.Created, "Acme's Alice is not Globex's contact")
	assert.NotEqual(t, int64(fixtures.ContactAliceID), res.Contact.ID)
	assert.EqualValues(t, fixtures.GlobexID, res.Contact.WorkspaceID)
}

func TestResolvePrefersSubjectThenEmailThenPhone(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	ctx := context.Background()

	bySubject, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{SubjectID: new("resolve-subject")})
	require.NoError(t, err)
	byPhone, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), contact.SourceManual, contacts.CreateInput{Phone: new("+15558880")})
	require.NoError(t, err)

	got, err := contacts.Resolve(ctx, env.DB.Scoped(fixtures.AcmeID), new(" resolve-subject "), lo.ToPtr(fixtures.ContactAliceEmail), nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, bySubject.ID, got.ID, "subject_id wins over email")

	got, err = contacts.Resolve(ctx, env.DB.Scoped(fixtures.AcmeID), new("unknown"), new("nobody@example.com"), new("+15558880"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, byPhone.ID, got.ID, "falls through to the next key")

	got, err = contacts.Resolve(ctx, env.DB.Scoped(fixtures.GlobexID), nil, lo.ToPtr(fixtures.ContactAliceEmail), nil)
	require.NoError(t, err)
	assert.Nil(t, got, "another workspace's contact does not resolve")

	got, err = contacts.Resolve(ctx, env.DB.Scoped(fixtures.AcmeID), nil, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, got, "no key resolves nothing")
}

func TestEnsureCustomFieldsWithoutValuesDeclaresNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.CustomField.Query().Count(context.Background())
	require.NoError(t, err)

	typed, err := contacts.EnsureCustomFields(context.Background(), env.DB.Scoped(fixtures.AcmeID), nil)
	require.NoError(t, err)
	assert.Nil(t, typed)
	after, err := env.DB.CustomField.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
