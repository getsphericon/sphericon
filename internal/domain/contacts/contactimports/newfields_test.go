package contactimports_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// newFieldImport is an import of Acme whose second column is mapped to a new number field
// `visits`, saved as role: the import and the refusal of the save.
func newFieldImport(t *testing.T, env *testhelper.TestEnv, role membership.Role) (*ent.ContactImport, error) {
	t.Helper()
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	uploaded, err := imports.Upload(ctx, acme, contactimports.UploadInput{File: strings.NewReader("email,visits\nann@example.com,3\n"), MembershipID: fixtures.AcmeOwnerMembershipID})
	require.NoError(t, err)
	_, saved := imports.SaveMapping(ctx, acme, uploaded.Import.ID, contactimports.MappingInput{Role: role, Mapping: csvimport.Mapping{
		Columns: []csvimport.Column{
			{Kind: csvimport.KindKey, Key: fieldname.Email},
			{Kind: csvimport.KindNewCustomField, Key: "visits", Label: "Visits", FieldType: string(customfield.TypeNumber)},
		},
		DecimalSeparator: csvimport.DecimalPoint,
	}})
	return uploaded.Import, saved
}

// visitsFields are the Custom fields of the key `visits` Acme has.
func visitsFields(t *testing.T, env *testhelper.TestEnv) int {
	t.Helper()
	n, err := env.DB.Scoped(fixtures.AcmeID).CustomField().Query().Where(customfield.Key("visits")).Count(t.Context())
	require.NoError(t, err)
	return n
}

// A Role without custom_fields:manage cannot map a column to a new Custom field: the save is
// refused with that Permission's refusal.
func TestMappingANewCustomFieldNeedsCustomFieldsManage(t *testing.T) {
	env := testhelper.Setup(t)

	imp, err := newFieldImport(t, env, membership.RoleMember)

	require.ErrorIs(t, err, memberships.Refusal(memberships.PermCustomFieldsManage))
	assert.Equal(t, contactimport.StateUploaded, imp.State)
}

// The run asks custom_fields:manage again, of the Member who runs it: one without it is refused,
// no field is created and the import stays ready; one with it creates the field before the job.
func TestRunningAnImportThatCreatesACustomFieldNeedsCustomFieldsManage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	imp, err := newFieldImport(t, env, membership.RoleAdmin)
	require.NoError(t, err)

	_, refused := imports.Run(ctx, acme, imp.ID, membership.RoleMember)
	afterRefusal := visitsFields(t, env)
	ready, err := imports.Get(ctx, acme, imp.ID)
	require.NoError(t, err)
	ran, err := imports.Run(ctx, acme, imp.ID, membership.RoleAdmin)
	require.NoError(t, err)

	require.ErrorIs(t, refused, memberships.Refusal(memberships.PermCustomFieldsManage))
	assert.Zero(t, afterRefusal)
	assert.Equal(t, contactimport.StateReady, ready.Import.State)
	assert.Equal(t, contactimport.StateRunning, ran.Import.State)
	assert.Equal(t, 1, visitsFields(t, env), "the field is created when the import runs")
}

// A key the Workspace takes after the mapping was saved (here declared by an ingest's use) refuses
// the run: the field it has is not retyped, and the import stays ready to be mapped again.
func TestRunningAnImportRefusesANewCustomFieldWhoseKeyWasTakenSinceSaving(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	imports := testhelper.Module[*contactimports.Module](env)
	imp, err := newFieldImport(t, env, membership.RoleOwner)
	require.NoError(t, err)
	_, err = testhelper.Module[*contacts.Module](env).EnsureCustomFields(ctx, acme, map[string]any{"visits": "many"})
	require.NoError(t, err)

	_, refused := imports.Run(ctx, acme, imp.ID, membership.RoleOwner)
	field, err := acme.CustomField().Query().Where(customfield.Key("visits")).Only(ctx)
	require.NoError(t, err)
	ready, err := imports.Get(ctx, acme, imp.ID)
	require.NoError(t, err)

	require.ErrorIs(t, refused, contacts.ErrCustomFieldKeyTaken)
	assert.Equal(t, customfield.TypeString, field.Type)
	assert.Equal(t, contactimport.StateReady, ready.Import.State)
}
