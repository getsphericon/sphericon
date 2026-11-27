package site_test

import (
	"encoding/csv"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// uploadCSV uploads file as an import of Acme.
func uploadCSV(t *testing.T, client *siteapi.Client, file string) (*siteapi.SiteContactImport, error) {
	t.Helper()
	return client.SiteContactImportsUpload(t.Context(),
		siteapi.SiteContactImportsUploadReq{Data: strings.NewReader(file)},
		siteapi.SiteContactImportsUploadParams{WorkspaceId: fixtures.AcmePublicID})
}

// uploaded is the import of file, uploaded by the owner.
func uploaded(t *testing.T, client *siteapi.Client, file string) *siteapi.SiteContactImport {
	t.Helper()
	imp, err := uploadCSV(t, client, file)
	require.NoError(t, err)
	return imp
}

// mappingInput is the mapping the import's answer shows, as a request saves it.
func mappingInput(t *testing.T, imp *siteapi.SiteContactImport) *siteapi.ContactImportMappingInput {
	t.Helper()
	in := testhelper.Resend[siteapi.ContactImportMappingInput](t, &imp.Mapping)
	return &in
}

// saveMapping saves in on the import.
func saveMapping(t *testing.T, client *siteapi.Client, imp *siteapi.SiteContactImport, in *siteapi.ContactImportMappingInput) (*siteapi.SiteContactImport, error) {
	t.Helper()
	return client.SiteContactImportsSaveMapping(t.Context(), in, siteapi.SiteContactImportsSaveMappingParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
}

// runImport saves in, runs the import and works its job: the import as it ended.
func runImport(t *testing.T, env *testhelper.TestEnv, client *siteapi.Client, imp *siteapi.SiteContactImport, in *siteapi.ContactImportMappingInput) *siteapi.SiteContactImport {
	t.Helper()
	_, err := saveMapping(t, client, imp, in)
	require.NoError(t, err)
	_, err = client.SiteContactImportsRun(t.Context(), siteapi.SiteContactImportsRunParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t)
	done, err := client.SiteContactImportsGet(t.Context(), siteapi.SiteContactImportsGetParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	require.NoError(t, err)
	return done
}

// contactByEmail is Acme's Contact of address with its Tags, nil when it has none.
func contactByEmail(t *testing.T, env *testhelper.TestEnv, address string) *ent.Contact {
	t.Helper()
	c, err := env.DB.Scoped(fixtures.AcmeID).Contact().Query().Where(contact.Email(address)).WithTags().Only(t.Context())
	if ent.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	return c
}

// Every operation of an import needs contacts:import (ADR 0138): a member is refused each one.
func TestAContactImportIsForMembersHoldingContactsImport(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	member := env.SiteActor(t, fixtures.MemberMaryEmail)
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := siteapi.EntityId(999999)

	_, upload := uploadCSV(t, member, "email\na@example.com\n")
	_, get := member.SiteContactImportsGet(ctx, siteapi.SiteContactImportsGetParams{WorkspaceId: ws, ImportId: id})
	_, save := member.SiteContactImportsSaveMapping(ctx, &siteapi.ContactImportMappingInput{Columns: []siteapi.ContactImportColumnInput{}, DecimalSeparator: siteapi.ContactImportDecimalSeparator_Dot, DefaultCountry: siteapi.NilCountryCode{Null: true}, Tag: siteapi.NilName{Null: true}}, siteapi.SiteContactImportsSaveMappingParams{WorkspaceId: ws, ImportId: id})
	_, preview := member.SiteContactImportsPreview(ctx, siteapi.SiteContactImportsPreviewParams{WorkspaceId: ws, ImportId: id})
	_, run := member.SiteContactImportsRun(ctx, siteapi.SiteContactImportsRunParams{WorkspaceId: ws, ImportId: id})
	_, link := member.SiteContactImportsReportLink(ctx, siteapi.SiteContactImportsReportLinkParams{WorkspaceId: ws, ImportId: id})
	_, report := member.SiteContactImportsReport(ctx, siteapi.SiteContactImportsReportParams{WorkspaceId: ws, ImportId: id, Token: "x"})

	for _, err := range []error{upload, get, save, preview, run, link, report} {
		refused := testhelper.SiteRefusal(t, err, http.StatusForbidden)
		assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.Code.Value)
	}
}

// A file saved with a byte order mark and semicolons uploads; its mapping is proposed from its
// header, the date column with the one format its sample reads in, numbers with a comma.
func TestAContactImportUploadsAFileWithABOMAndSemicolons(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	imp := uploaded(t, owner, "\xEF\xBB\xBFEmail;First name;birthday;Notes;email\nann@example.com;Ann;1990-02-03;;dup@example.com\nbob@example.com;Bob\n")

	assert.Equal(t, siteapi.ContactImportStateUploaded, imp.State)
	assert.Equal(t, siteapi.ContactImportDelimiterSemicolon, imp.Delimiter)
	assert.Equal(t, []string{"Email", "First name", "birthday", "Notes", "email"}, imp.Header)
	assert.Equal(t, int32(2), imp.Rows)
	assert.Equal(t, int32(1), imp.MalformedRows)
	assert.Equal(t, siteapi.ContactImportDecimalSeparator_, imp.Mapping.DecimalSeparator)
	require.Len(t, imp.Mapping.Columns, 5)
	assert.Equal(t, siteapi.ContactImportKeyEmail, imp.Mapping.Columns[0].OneOf.ContactImportKeyColumn.Key)
	assert.Equal(t, siteapi.ContactImportAttributeFirstName, imp.Mapping.Columns[1].OneOf.ContactImportAttributeColumn.Attribute)
	assert.Equal(t, siteapi.ContactImportDateFormatYyyyMinusMmMinusDd, imp.Mapping.Columns[2].OneOf.ContactImportAttributeColumn.DateFormat.Value)
	assert.Equal(t, siteapi.ContactImportIgnoreColumnContactImportColumnSum, imp.Mapping.Columns[3].OneOf.Type)
	assert.Equal(t, siteapi.ContactImportIgnoreColumnContactImportColumnSum, imp.Mapping.Columns[4].OneOf.Type, "a repeated header is ignored")
	assert.True(t, strings.HasPrefix(string(imp.Mapping.Tag.Value), "Imported "))
}

// A file that is not UTF-8 and one of more columns than an import maps are refused whole, each with
// its reason.
func TestAContactImportRefusesAFileNotUTF8OrTooWide(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, latin1 := uploadCSV(t, owner, "email\nfran\xe7ois@example.com\n")
	_, wide := uploadCSV(t, owner, strings.Repeat("c,", 200)+"c\n")

	notUTF8 := testhelper.SiteRefusal(t, latin1, http.StatusUnprocessableEntity)
	assert.Contains(t, notUTF8.Detail.Value, "UTF-8")
	tooWide := testhelper.SiteRefusal(t, wide, http.StatusUnprocessableEntity)
	assert.Contains(t, tooWide.Detail.Value, "200 columns")
}

// A file over the Instance's import limit is refused with the limit, whatever the default body cap.
func TestAContactImportRefusesAFileOverTheLimit(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.BodyLimits.CSVImport = 64 }))
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, small := uploadCSV(t, owner, "email\na@example.com\n")
	_, large := uploadCSV(t, owner, "email\n"+strings.Repeat("someone@example.com\n", 4))

	require.NoError(t, small)
	tooLarge := testhelper.SiteRefusal(t, large, http.StatusRequestEntityTooLarge)
	assert.Contains(t, tooLarge.Detail.Value, "64 bytes")
}

// The preview shows the first rows as the import would write them: a birthday not in its column's
// format is dropped, a row without a key refused; over the file it counts the rows without a key
// and the distinct Tags.
func TestAContactImportPreviewShowsDroppedValuesAndRefusedRows(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,first_name,birthday,tags\nann@example.com,Ann,31/02/1990,vip\n,Nobody,01/02/1990,\"vip,Newsletter\"\n")
	in := mappingInput(t, imp)
	in.Columns[2] = siteapi.ContactImportColumnInput{OneOf: siteapi.NewContactImportAttributeColumnInputContactImportColumnInputSum(siteapi.ContactImportAttributeColumnInput{
		Kind: siteapi.ContactImportAttributeColumnInputKindAttribute, Attribute: siteapi.ContactImportAttributeBirthday,
		DateFormat: siteapi.NewNilContactImportDateFormat(siteapi.ContactImportDateFormatDdSlashMmSlashYyyy),
	})}
	in.Tag = siteapi.NewNilName("From the file")
	_, err := saveMapping(t, owner, imp, in)
	require.NoError(t, err)

	preview, err := owner.SiteContactImportsPreview(t.Context(), siteapi.SiteContactImportsPreviewParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	require.NoError(t, err)

	require.Len(t, preview.Rows, 2)
	assert.Equal(t, []string{"ann@example.com", "Ann", "31/02/1990", "vip"}, preview.Rows[0].Cells)
	assert.True(t, preview.Rows[0].Refusal.Null)
	require.Len(t, preview.Rows[0].Dropped, 1)
	assert.Equal(t, siteapi.ContactImportIssueReasonNotDate, preview.Rows[0].Dropped[0].Reason)
	assert.Equal(t, int32(2), preview.Rows[0].Dropped[0].Column.Value)
	assert.Equal(t, siteapi.ContactImportIssueReasonNoKey, preview.Rows[1].Refusal.Value.Reason)
	assert.Equal(t, int32(2), preview.TotalRows)
	assert.Equal(t, int32(1), preview.RowsWithoutKey)
	assert.Equal(t, int32(2), preview.DistinctTags, "vip and the Tag for every row")
}

// A date column whose sample reads in two formats gets none: its mapping cannot be saved, so the
// import cannot run, until the Member chooses one.
func TestAnAmbiguousDateColumnBlocksTheRunUntilAFormatIsChosen(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,birthday\nann@example.com,03/04/1990\n")
	in := mappingInput(t, imp)

	_, unsaved := saveMapping(t, owner, imp, in)
	_, notReady := owner.SiteContactImportsRun(ctx, siteapi.SiteContactImportsRunParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID})
	in.Columns[1].OneOf.ContactImportAttributeColumnInput.DateFormat = siteapi.NewNilContactImportDateFormat(siteapi.ContactImportDateFormatMmSlashDdSlashYyyy)
	done := runImport(t, env, owner, imp, in)
	ann := contactByEmail(t, env, "ann@example.com")

	assert.True(t, imp.Mapping.Columns[1].OneOf.ContactImportAttributeColumn.DateFormat.Null, "no format is proposed")
	testhelper.SiteRefusal(t, unsaved, http.StatusUnprocessableEntity)
	testhelper.SiteRefusal(t, notReady, http.StatusConflict)
	assert.Equal(t, siteapi.ContactImportStateDone, done.State)
	require.NotNil(t, ann)
	assert.Equal(t, time.Date(1990, 3, 4, 0, 0, 0, 0, time.UTC), lo.FromPtr(ann.Birthday))
}

// The run writes the rows as the Member who ran it: a new Contact has the source `import`, its Tags
// and the Tag for every row; a blank cell leaves an existing Contact's value; the whole job is one
// contact.import Audit entry.
func TestAContactImportWritesItsRows(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,first_name,last_name,tags\nnew@example.com,New,Person,\"vip, newsletter\"\n"+fixtures.ContactAliceEmail+",,Jones,\n")
	in := mappingInput(t, imp)
	in.Tag = siteapi.NewNilName("From Mailchimp")

	done := runImport(t, env, owner, imp, in)
	fresh := contactByEmail(t, env, "new@example.com")
	alice := contactByEmail(t, env, fixtures.ContactAliceEmail)
	imports := lo.Filter(testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry), func(e *events.AuditEntry, _ int) bool {
		return e.Action == events.ActionContactImport
	})

	assert.Equal(t, siteapi.ContactImportStateDone, done.State)
	assert.Equal(t, siteapi.ContactImportCounts{Processed: 2, Created: 1, Updated: 1}, done.Counts)
	require.NotNil(t, fresh)
	assert.Equal(t, contact.SourceImport, fresh.Source)
	assert.ElementsMatch(t, []string{"vip", "newsletter", "From Mailchimp"}, lo.Map(fresh.Edges.Tags, func(tg *ent.Tag, _ int) string { return tg.Name }))
	assert.Equal(t, "Alice", lo.FromPtr(alice.FirstName), "a blank cell changes nothing")
	assert.Equal(t, "Jones", lo.FromPtr(alice.LastName))
	require.Len(t, imports, 1)
	assert.Equal(t, events.ActorUser, imports[0].Actor.Kind)
}

// "Only add new Contacts" leaves an existing Contact as it was and creates the new ones.
func TestAContactImportThatOnlyAddsNewLeavesAnExistingContactUnchanged(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	imp := uploaded(t, owner, "email,first_name,tags\n"+fixtures.ContactAliceEmail+",Alicia,from-the-file\nneo@example.com,Neo,from-the-file\n")
	in := mappingInput(t, imp)
	in.OnlyAddNew = true

	done := runImport(t, env, owner, imp, in)
	alice := contactByEmail(t, env, fixtures.ContactAliceEmail)
	neo := contactByEmail(t, env, "neo@example.com")

	assert.Equal(t, siteapi.ContactImportCounts{Processed: 2, Created: 1, Updated: 1}, done.Counts)
	assert.Equal(t, "Alice", lo.FromPtr(alice.FirstName))
	assert.NotContains(t, lo.Map(alice.Edges.Tags, func(tg *ent.Tag, _ int) string { return tg.Name }), "from-the-file")
	require.NotNil(t, neo)
	assert.Equal(t, "Neo", lo.FromPtr(neo.FirstName))
}

// readReport is the CSV a report download streams, its byte order mark dropped.
func readReport(t *testing.T, res *siteapi.SiteContactImportsReportOKHeaders) [][]string {
	t.Helper()
	raw, err := io.ReadAll(res.Response)
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\xEF\xBB\xBF"))).ReadAll()
	require.NoError(t, err)
	return records
}

// The refused rows' report downloads once, in the session that asked for its link, as CSV of the
// original cells with a column saying why; the link fails for another session and a second time,
// and the report is gone once downloaded.
func TestAContactImportReportDownloadsOnceInItsSession(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	session := env.SiteWithToken(t, env.SiteToken(t, fixtures.OwnerJohnEmail, nil))
	other := env.SiteWithToken(t, env.SiteToken(t, fixtures.OwnerJohnEmail, func(c *apiauth.SessionClaims) {
		c.IssuedAt = jwt.NewNumericDate(c.IssuedAt.Add(-time.Minute))
	}))
	imp := uploaded(t, session, "email,first_name\nann@example.com,Ann\n,=Nobody\nnot an address,Bad\n")
	done := runImport(t, env, session, imp, mappingInput(t, imp))
	linkParams := siteapi.SiteContactImportsReportLinkParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID}

	link, err := session.SiteContactImportsReportLink(ctx, linkParams)
	require.NoError(t, err)
	u, err := url.Parse(link.URL)
	require.NoError(t, err)
	params := siteapi.SiteContactImportsReportParams{WorkspaceId: fixtures.AcmePublicID, ImportId: imp.ID, Token: u.Query().Get("token")}
	_, elsewhere := other.SiteContactImportsReport(ctx, params)
	res, err := session.SiteContactImportsReport(ctx, params)
	require.NoError(t, err)
	records := readReport(t, res)
	_, again := session.SiteContactImportsReport(ctx, params)
	_, gone := session.SiteContactImportsReportLink(ctx, linkParams)

	assert.Equal(t, int32(2), done.ReportRows)
	assert.Contains(t, res.ContentDisposition, "attachment")
	testhelper.SiteRefusal(t, elsewhere, http.StatusBadRequest)
	require.Len(t, records, 3)
	assert.Equal(t, []string{"email", "first_name", "Issues"}, records[0])
	assert.Equal(t, []string{"", "'=Nobody"}, records[1][:2], "a cell a spreadsheet would run is neutralised")
	assert.Contains(t, records[1][2], "no subject ID, email or phone")
	assert.Equal(t, "not an address", records[2][0])
	assert.NotEmpty(t, records[2][2])
	testhelper.SiteRefusal(t, again, http.StatusBadRequest)
	testhelper.SiteRefusal(t, gone, http.StatusNotFound)
}
