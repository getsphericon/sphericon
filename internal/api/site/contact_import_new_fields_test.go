package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/customfield"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// newFieldColumn is a column mapped to a new Custom field of key, label and type t, its dates
// written in format (none when null).
func newFieldColumn(key, label string, t siteapi.CustomFieldType, format siteapi.NilContactImportDateFormat) siteapi.ContactImportColumnInput {
	return siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportNewCustomFieldColumnInputContactImportColumnInputSum(siteapi.ContactImportNewCustomFieldColumnInput{
		Kind: siteapi.ContactImportNewCustomFieldColumnInputKindNewCustomField, Key: key, Label: siteapi.Name(label), Type: t, DateFormat: format,
	})}
}

// A column mapped to a new datetime field read day first: the run creates the field with its label
// and type, stores the dates it reads, and drops and reports the one it cannot read.
func TestAContactImportCreatesANewCustomFieldOfTheChosenType(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,joined\nann@example.com,05/03/2024\nzed@example.com,2024-13-45\n")
	in := mappingInput(t, imp)
	in.Columns[1] = newFieldColumn("joined_on", "Joined on", siteapi.CustomFieldTypeDatetime, siteapi.NewNilContactImportDateFormat(siteapi.ContactImportDateFormatDdSlashMmSlashYyyy))

	done := runImport(t, env, owner, imp, in)
	field, err := env.DB.Scoped(fixtures.AcmeID).CustomField().Query().Where(customfield.Key("joined_on")).Only(t.Context())
	require.NoError(t, err)
	ann := contactByEmail(t, env, "ann@example.com")
	bob := contactByEmail(t, env, "zed@example.com")

	assert.Equal(t, "Joined on", field.Name)
	assert.Equal(t, customfield.TypeDatetime, field.Type)
	assert.Equal(t, siteapi.ContactImportCounts{Processed: 2, Created: 2, Dropped: 1}, done.Counts)
	assert.Equal(t, "2024-03-05", ann.CustomFields["joined_on"], "read day first")
	assert.NotContains(t, bob.CustomFields, "joined_on")
	assert.Equal(t, int32(1), done.ReportRows, "the value not read is reported")
	assert.Equal(t, siteapi.ContactImportNewCustomFieldColumnKindNewCustomField, done.Mapping.Columns[1].OneOf.ContactImportNewCustomFieldColumn.Kind)
}

// A new field's key the Workspace has a field of already is refused when the mapping is saved: the
// Member maps the column to that field instead.
func TestAContactImportRefusesANewCustomFieldOfAKeyTaken(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,plan\nann@example.com,gold\n")
	in := mappingInput(t, imp)
	in.Columns[1] = newFieldColumn(fixtures.CustomFieldAcmePlanKey, "Plan", siteapi.CustomFieldTypeString, siteapi.NilContactImportDateFormat{Null: true})
	padded := mappingInput(t, imp)
	padded.Columns[1] = newFieldColumn(" "+fixtures.CustomFieldAcmePlanKey+" ", "Plan", siteapi.CustomFieldTypeString, siteapi.NilContactImportDateFormat{Null: true})

	_, err := saveMapping(t, owner, imp, in)
	_, paddedErr := saveMapping(t, owner, imp, padded)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Contains(t, refused.Detail.Value, fixtures.CustomFieldAcmePlanKey)
	testhelper.SiteRefusal(t, paddedErr, http.StatusConflict, "a key is read without its spaces")
}

// The preview reads a new field's column in its chosen type before any field exists, counts the
// fields the run would create, and creates none.
func TestAContactImportPreviewCountsTheNewCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,score,vip\nann@example.com,abc,yes\n")
	in := mappingInput(t, imp)
	in.Columns[1] = newFieldColumn("score", "Score", siteapi.CustomFieldTypeNumber, siteapi.NilContactImportDateFormat{Null: true})
	in.Columns[2] = newFieldColumn("vip", "VIP", siteapi.CustomFieldTypeBool, siteapi.NilContactImportDateFormat{Null: true})
	_, err := saveMapping(t, owner, imp, in)
	require.NoError(t, err)

	preview, err := owner.SiteContactImportsPreview(t.Context(), siteapi.SiteContactImportsPreviewParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	require.NoError(t, err)
	created, err := env.DB.Scoped(fixtures.AcmeID).CustomField().Query().Where(customfield.KeyIn("score", "vip")).Count(t.Context())
	require.NoError(t, err)

	assert.Equal(t, int32(2), preview.NewCustomFields)
	require.Len(t, preview.Rows, 1)
	require.Len(t, preview.Rows[0].Dropped, 1)
	assert.Equal(t, siteapi.ContactImportIssueReasonNotNumber, preview.Rows[0].Dropped[0].Reason)
	assert.True(t, preview.Rows[0].Refusal.Null)
	assert.Zero(t, created, "a preview writes nothing")
}
