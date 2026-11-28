package site_test

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// acmeExport is the CSV export of every Acme Contact with every column, as its owner downloads it.
func acmeExport(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	token := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{Columns: siteapi.ContactCsvColumn("").AllValues()})
	res, err := followCSVExport(t, owner, token)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Response.Data)
	require.NoError(t, err)
	return string(body)
}

// importIntoGlobex uploads file into Globex as its owner, saves the mapping the app proposes as it
// is, runs the import and works its job: the import as it ended.
func importIntoGlobex(t *testing.T, env *testhelper.TestEnv, file string) *siteapi.SiteContactImport {
	t.Helper()
	ctx := t.Context()
	owner := env.SiteActor(t, fixtures.OwnerJaneEmail)
	imp, err := owner.SiteContactImportsUpload(ctx, siteapi.SiteContactImportsUploadReq{Data: strings.NewReader(file)},
		siteapi.SiteContactImportsUploadParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	in := mappingInput(t, imp)
	_, err = owner.SiteContactImportsSaveMapping(ctx, in, siteapi.SiteContactImportsSaveMappingParams{WorkspaceId: fixtures.GlobexPublicID, ImportId: imp.ID})
	require.NoError(t, err, "the proposal saves as it is")
	_, err = owner.SiteContactImportsRun(ctx, siteapi.SiteContactImportsRunParams{WorkspaceId: fixtures.GlobexPublicID, ImportId: imp.ID})
	require.NoError(t, err, "the proposal runs as it is")
	env.PerformEnqueuedJobs(t)
	done, err := owner.SiteContactImportsGet(ctx, siteapi.SiteContactImportsGetParams{WorkspaceId: fixtures.GlobexPublicID, ImportId: imp.ID})
	require.NoError(t, err)
	return done
}

// globexStates are the Channel states of Globex's Contacts, by the Contact's email, else phone.
func globexStates(t *testing.T, env *testhelper.TestEnv) (byKey map[string]*ent.Contact, statesByKey map[string]eligibility.ContactStates) {
	t.Helper()
	globex := env.DB.Scoped(fixtures.GlobexID)
	contacts, err := globex.Contact().Query().WithTags().All(t.Context())
	require.NoError(t, err)
	states, err := eligibility.New().ChannelStates(t.Context(), globex, contacts)
	require.NoError(t, err)
	key := func(c *ent.Contact) string { return lo.CoalesceOrEmpty(lo.FromPtr(c.Email), lo.FromPtr(c.Phone)) }
	byKey = lo.SliceToMap(contacts, func(c *ent.Contact) (string, *ent.Contact) { return key(c), c })
	statesByKey = lo.SliceToMap(contacts, func(c *ent.Contact) (string, eligibility.ContactStates) { return key(c), states[c.ID] })
	return byKey, statesByKey
}

// scopes are the scopes a Channel state is opted out of.
func scopes(st eligibility.ChannelState) []string {
	return lo.Map(st.OptOuts, func(o eligibility.OptOut, _ int) string { return o.Scope })
}

// A Workspace's export imports into another Workspace as it is (ADR 0138): the mapping the app
// proposes from the export's headers saves and runs unchanged, and brings the Contacts with their
// keys, attributes, Tags and Custom fields (created in their types), the opt-outs of everything and
// of broadcasts (off email, where a person stops everything, as one of everything), each
// Suppression as a manual one and each Confirmation as an imported one; an opt-out of a single
// Automation comes back as nothing.
func TestAnExportImportsIntoAnotherWorkspaceAsItIs(t *testing.T) {
	env := testhelper.Setup(t)
	file := acmeExport(t, env)

	done := importIntoGlobex(t, env, file)
	contacts, states := globexStates(t, env)
	fields, err := env.DB.Scoped(fixtures.GlobexID).CustomField().Query().All(t.Context())
	require.NoError(t, err)
	types := lo.SliceToMap(fields, func(f *ent.CustomField) (string, customfield.Type) { return f.Key, f.Type })

	require.Equal(t, siteapi.ContactImportStateDone, done.State)
	assert.Equal(t, customfield.TypeString, types["plan"])
	assert.Equal(t, customfield.TypeNumber, types["courses_completed"])
	assert.Equal(t, customfield.TypeBool, types["is_trial"])
	alice := contacts[fixtures.ContactAliceEmail]
	require.NotNil(t, alice)
	assert.Equal(t, contact.SourceImport, alice.Source)
	assert.Equal(t, "Smith", lo.FromPtr(alice.LastName))
	assert.Subset(t, lo.Map(alice.Edges.Tags, func(tg *ent.Tag, _ int) string { return tg.Name }), []string{"vip", "newsletter"})
	subject := contacts["export.subject@example.com"]
	require.NotNil(t, subject)
	assert.Equal(t, "+14155550800", lo.FromPtr(subject.Phone), "the phone the export guarded with an apostrophe reads back")
	assert.Equal(t, "user:export-subject", lo.FromPtr(subject.SubjectID))
	assert.Equal(t, "Europe/Berlin", lo.FromPtr(subject.TimeZone))
	assert.Equal(t, []string{eligibility.SourceEverything}, scopes(states["scarlett.sql@codebasics.dev"].Email))
	assert.Equal(t, []string{eligibility.SourceBroadcasts}, scopes(states[fixtures.ContactBobEmail].Email))
	assert.Empty(t, scopes(states["leo.devops@codebasics.dev"].Email), "an opt-out of one Automation means nothing in another Workspace")
	assert.Equal(t, eligibility.StateSuppressed, states[fixtures.ContactBouncedEmail].Email.State)
	require.NotNil(t, states[fixtures.ContactBouncedEmail].Email.Suppression)
	assert.Equal(t, entsuppression.ReasonManual, states[fixtures.ContactBouncedEmail].Email.Suppression.Reason)
	require.NotNil(t, states["liam.python@codebasics.dev"].Email.Confirmation)
	assert.Equal(t, confirmation.ProvenanceImported, states["liam.python@codebasics.dev"].Email.Confirmation.Provenance)
	assert.Equal(t, []string{eligibility.SourceEverything}, scopes(states["+13125557505"].SMS))
	assert.Equal(t, []string{eligibility.SourceEverything}, scopes(states["+13125557504"].SMS), "off email an opt-out of broadcasts comes back as one of everything")
}

// proposedKinds are the kinds of the columns an upload's proposal maps, by the file's header.
func proposedKinds(t *testing.T, imp *siteapi.SiteContactImport) map[string]csvimport.Kind {
	t.Helper()
	header := imp.Header
	return lo.SliceToMap(lo.Range(len(header)), func(i int) (string, csvimport.Kind) {
		raw, err := imp.Mapping.Columns[i].MarshalJSON()
		require.NoError(t, err)
		var col struct {
			Kind csvimport.Kind `json:"kind"`
		}
		require.NoError(t, json.Unmarshal(raw, &col))
		return header[i], col.Kind
	})
}

// Every header an export writes is proposed as what it means, or deliberately ignored: the
// Contact's source and timestamps and each Channel's state are the exporting Workspace's own, and
// Telegram has no Confirmation an import records.
func TestEveryHeaderOfAnExportIsProposed(t *testing.T) {
	env := testhelper.Setup(t)
	file := acmeExport(t, env)
	owner := env.SiteActor(t, fixtures.OwnerJaneEmail)

	imp, err := owner.SiteContactImportsUpload(t.Context(), siteapi.SiteContactImportsUploadReq{Data: strings.NewReader(file)},
		siteapi.SiteContactImportsUploadParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	kinds := proposedKinds(t, imp)

	assert.Equal(t, map[string]csvimport.Kind{
		string(siteapi.ContactCsvColumnSubjectId): csvimport.KindKey, string(siteapi.ContactCsvColumnEmail): csvimport.KindKey, string(siteapi.ContactCsvColumnPhone): csvimport.KindKey,
		string(siteapi.ContactCsvColumnFirstName): csvimport.KindAttribute, string(siteapi.ContactCsvColumnLastName): csvimport.KindAttribute, string(siteapi.ContactCsvColumnTimeZone): csvimport.KindAttribute, string(siteapi.ContactCsvColumnCountry): csvimport.KindAttribute,
		string(siteapi.ContactCsvColumnLocale): csvimport.KindAttribute, string(siteapi.ContactCsvColumnBirthday): csvimport.KindAttribute, string(siteapi.ContactCsvColumnCity): csvimport.KindAttribute, string(siteapi.ContactCsvColumnAvatar): csvimport.KindAttribute,
		string(siteapi.ContactCsvColumnSource): csvimport.KindIgnore, string(siteapi.ContactCsvColumnCreatedAt): csvimport.KindIgnore, string(siteapi.ContactCsvColumnUpdatedAt): csvimport.KindIgnore,
		"customFields.courses_completed": csvimport.KindNewCustomField, "customFields.is_trial": csvimport.KindNewCustomField,
		"customFields.plan": csvimport.KindNewCustomField, "customFields.renewal_on": csvimport.KindNewCustomField,
		"customFields.signup_source": csvimport.KindNewCustomField, "customFields.trial_ends_at": csvimport.KindNewCustomField,
		string(siteapi.ContactCsvColumnTags): csvimport.KindTags,
		"email.state":                        csvimport.KindIgnore, "email.optedOutScopes": csvimport.KindRefusal, "email.optedOutAt": csvimport.KindTime,
		"email.suppressionReason": csvimport.KindRefusal, "email.confirmation": csvimport.KindConsent, "email.confirmedAt": csvimport.KindTime,
		"sms.state": csvimport.KindIgnore, "sms.optedOutScopes": csvimport.KindRefusal, "sms.optedOutAt": csvimport.KindTime,
		"sms.suppressionReason": csvimport.KindRefusal, "sms.confirmation": csvimport.KindConsent, "sms.confirmedAt": csvimport.KindTime,
		"telegram.state": csvimport.KindIgnore, "telegram.optedOutScopes": csvimport.KindRefusal, "telegram.optedOutAt": csvimport.KindTime,
		"telegram.suppressionReason": csvimport.KindRefusal, "telegram.confirmation": csvimport.KindIgnore, "telegram.confirmedAt": csvimport.KindIgnore,
	}, kinds)
}

// A Custom field header of an export maps to the Workspace's field of its key, and a cell the
// export guarded with an apostrophe reads back as it was written, while an apostrophe a cell begins
// with otherwise is the cell's own. A new field is typed as its cells read once read back.
func TestAnExportsGuardedCellsReadBackAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,customFields.courses_completed,customFields.signup_source,customFields.score\n"+
		"ann.moved@example.com,'-3,'hello,'-1.5\nbob.moved@example.com,2,'=x,4\n")
	kinds := proposedKinds(t, imp)

	runImport(t, env, owner, imp, mappingInput(t, imp))
	ann := contactByEmail(t, env, "ann.moved@example.com")
	bob := contactByEmail(t, env, "bob.moved@example.com")
	score, err := env.DB.Scoped(fixtures.AcmeID).CustomField().Query().Where(customfield.Key("score")).Only(t.Context())
	require.NoError(t, err)

	assert.Equal(t, csvimport.KindCustomField, kinds["customFields.courses_completed"], "the Workspace's own field")
	assert.Equal(t, customfield.TypeNumber, score.Type)
	require.NotNil(t, ann)
	assert.Equal(t, map[string]any{"courses_completed": -3.0, "signup_source": "'hello", "score": -1.5}, ann.CustomFields)
	require.NotNil(t, bob)
	assert.Equal(t, "=x", bob.CustomFields["signup_source"])
}
