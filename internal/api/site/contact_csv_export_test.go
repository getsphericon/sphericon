package site_test

import (
	"encoding/csv"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// csvExportLink asks for a CSV export of Contacts and returns the token of its link.
func csvExportLink(t *testing.T, c *siteapi.Client, in siteapi.SiteContactCsvExportInput) string {
	t.Helper()
	link, err := c.SiteContactCsvExportsCreate(t.Context(), &in, siteapi.SiteContactCsvExportsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Equal(t, "/site/workspaces/"+string(fixtures.AcmePublicID)+"/contact-exports", path.Dir(link.URL), "the link is a path of /site")
	return path.Base(link.URL)
}

// followCSVExport follows the link of a CSV export.
func followCSVExport(t *testing.T, c *siteapi.Client, token string) (*siteapi.SiteContactCsvExportsDownloadOKHeaders, error) {
	t.Helper()
	return c.SiteContactCsvExportsDownload(t.Context(), siteapi.SiteContactCsvExportsDownloadParams{WorkspaceId: fixtures.AcmePublicID, Token: token})
}

// csvExportRows reads the streamed file: one BOM, then the records.
func csvExportRows(t *testing.T, res *siteapi.SiteContactCsvExportsDownloadOKHeaders) [][]string {
	t.Helper()
	assert.Contains(t, res.ContentDisposition, "attachment")
	assert.Contains(t, res.ContentDisposition, ".csv")
	body, err := io.ReadAll(res.Response.Data)
	require.NoError(t, err)
	file, found := strings.CutPrefix(string(body), "\xEF\xBB\xBF")
	require.True(t, found, "the file begins with a UTF-8 BOM")
	rows, err := csv.NewReader(strings.NewReader(file)).ReadAll()
	require.NoError(t, err)
	return rows
}

// byEmail is the row of the file whose email column holds the address.
func byEmail(t *testing.T, rows [][]string, email string) map[string]string {
	t.Helper()
	header := rows[0]
	at := slices.Index(header, "email")
	require.GreaterOrEqual(t, at, 0, "the file has an email column")
	row, found := lo.Find(rows[1:], func(r []string) bool { return r[at] == email })
	require.True(t, found, "a row of %s", email)
	return lo.SliceToMap(lo.Range(len(header)), func(i int) (string, string) { return header[i], row[i] })
}

// A Segment's export is a CSV file of its members with the chosen columns, in their order: the
// attributes under the contract's names, a column per Custom field, and the Tags as one record.
func TestContactCsvExportOfASegmentHasTheChosenColumns(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	token := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{
		SegmentId: siteapi.NewOptEntityId(fixtures.SegmentProPlanID),
		Columns: []siteapi.ContactCsvColumn{
			siteapi.ContactCsvColumnEmail, siteapi.ContactCsvColumnFirstName, siteapi.ContactCsvColumnBirthday,
			siteapi.ContactCsvColumnCustomFields, siteapi.ContactCsvColumnTags, siteapi.ContactCsvColumnEmail,
		},
	})
	res, err := followCSVExport(t, owner, token)
	require.NoError(t, err)
	rows := csvExportRows(t, res)

	header := rows[0]
	assert.Equal(t, []string{"email", "firstName", "birthday"}, header[:3], "the columns in the order chosen, a repeated one once")
	assert.Equal(t, "tags", header[len(header)-1])
	assert.Subset(t, header, []string{"customFields.plan", "customFields.courses_completed", "customFields.is_trial"})
	liam := byEmail(t, rows, "liam.python@codebasics.dev")
	assert.Equal(t, "Liam", liam["firstName"])
	assert.Equal(t, "1994-03-14", liam["birthday"])
	assert.Equal(t, "pro", liam["customFields.plan"])
	assert.Equal(t, "3", liam["customFields.courses_completed"])
	assert.Equal(t, "false", liam["customFields.is_trial"])
	plans := lo.Map(rows[1:], func(r []string, _ int) string { return r[slices.Index(header, "customFields.plan")] })
	assert.NotEmpty(t, plans)
	assert.Equal(t, []string{"pro"}, lo.Uniq(plans), "only the Segment's members")
}

// Without a Segment the export is every Contact of the Workspace; a Contact's Tags are one CSV
// record of their names.
func TestContactCsvExportOfAllContactsHasEachContact(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	token := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{
		Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail, siteapi.ContactCsvColumnSubjectId, siteapi.ContactCsvColumnTags},
	})
	res, err := followCSVExport(t, owner, token)
	require.NoError(t, err)
	rows := csvExportRows(t, res)

	subject := byEmail(t, rows, fixtures.ContactExportSubjectEmail)
	assert.Equal(t, "user:export-subject", subject["subjectId"])
	assert.Equal(t, "newsletter,vip", subject["tags"])
	assert.NotEmpty(t, byEmail(t, rows, "liam.python@codebasics.dev"), "a Contact of no Segment in particular")
}

// The link works once.
func TestContactCsvExportLinkWorksOnce(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	token := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail}})
	_, err := followCSVExport(t, owner, token)
	require.NoError(t, err)

	_, err = followCSVExport(t, owner, token)

	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
}

// The link works only for the session that asked for it: the same User signed in elsewhere is
// refused.
func TestContactCsvExportLinkIsForItsSession(t *testing.T) {
	clock := testhelper.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	token := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail}})
	elsewhere := env.SiteWithToken(t, env.SiteToken(t, fixtures.OwnerJohnEmail, func(c *apiauth.SessionClaims) {
		c.IssuedAt = jwt.NewNumericDate(clock.Now().Add(-time.Hour))
	}))

	_, err := followCSVExport(t, elsewhere, token)
	testhelper.SiteRefusal(t, err, http.StatusBadRequest)

	_, err = followCSVExport(t, owner, token)
	require.NoError(t, err, "the refusal did not spend the link")
}

// The link lapses a minute after it is issued.
func TestContactCsvExportLinkLapsesAfterAMinute(t *testing.T) {
	clock := testhelper.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	owner := env.SiteWithToken(t, env.SiteToken(t, fixtures.OwnerJohnEmail, nil))
	early := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail}})
	late := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail}})

	clock.Advance(59 * time.Second)
	_, err := followCSVExport(t, owner, early)
	require.NoError(t, err, "within the minute")

	clock.Advance(time.Second)
	_, err = followCSVExport(t, owner, late)
	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
}

// The bulk export is the Permission contacts:export: a plain member can neither ask for it nor
// follow a link.
func TestContactCsvExportIsForOwnersAndAdmins(t *testing.T) {
	env := testhelper.Setup(t)
	token := csvExportLink(t, env.SiteActor(t, fixtures.AdminAliceEmail), siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail}})
	member := env.SiteActor(t, fixtures.MemberMaryEmail)

	_, err := member.SiteContactCsvExportsCreate(t.Context(), &siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail}},
		siteapi.SiteContactCsvExportsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	refused := testhelper.SiteRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.Code.Value)

	_, err = followCSVExport(t, member, token)
	refused = testhelper.SiteRefusal(t, err, http.StatusForbidden)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.Code.Value)
}

// An export of a Segment the Workspace does not have is not found; one of no column is refused.
func TestContactCsvExportRefusesAnUnknownSegmentAndNoColumn(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteContactCsvExportsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	_, err := owner.SiteContactCsvExportsCreate(t.Context(), &siteapi.SiteContactCsvExportInput{
		SegmentId: siteapi.NewOptEntityId(fixtures.SegmentTyrellTextableID),
		Columns:   []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail},
	}, params)
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "another Workspace's Segment")

	_, err = owner.SiteContactCsvExportsCreate(t.Context(), &siteapi.SiteContactCsvExportInput{Columns: []siteapi.ContactCsvColumn{}}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// Following the link records one contact.export entry naming the Segment, the columns and the
// rows, before the first row leaves; asking for the link records nothing.
func TestContactCsvExportIsAuditedAsItStarts(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	token := csvExportLink(t, owner, siteapi.SiteContactCsvExportInput{
		SegmentId: siteapi.NewOptEntityId(fixtures.SegmentProPlanID),
		Columns:   []siteapi.ContactCsvColumn{siteapi.ContactCsvColumnEmail, siteapi.ContactCsvColumnEmailChannel},
	})
	asked := exportEntries(t, env)
	require.Empty(t, asked)

	res, err := followCSVExport(t, owner, token)
	require.NoError(t, err)
	entries := exportEntries(t, env)
	rows := csvExportRows(t, res)

	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "segment", e.TargetType)
	assert.Equal(t, publicid.Encode(fixtures.SegmentProPlanID), e.TargetID)
	assert.Equal(t, "Pro & team members", e.TargetName)
	assert.Equal(t, "John", e.Actor.Name)
	assert.ElementsMatch(t, []any{"email", "emailChannel"}, e.Diff["columns"])
	assert.EqualValues(t, len(rows)-1, e.Diff["rows"], "the rows of the file")
}

// exportEntries are the contact.export Audit entries the test queued.
func exportEntries(t *testing.T, env *testhelper.TestEnv) []*events.AuditEntry {
	t.Helper()
	return lo.Filter(testhelper.Queued[*events.AuditEntry](t, env, events.NameAuditEntry), func(e *events.AuditEntry, _ int) bool {
		return e.Action == events.ActionContactExport
	})
}

// The one-Contact export is audited too, naming the Contact.
func TestSiteContactsExportIsAudited(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteContactsExport(t.Context(), siteapi.SiteContactsExportParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          siteapi.NewOptEntityId(siteapi.EntityId(fixtures.ContactExportSubjectID)),
	})
	require.NoError(t, err)
	entries := exportEntries(t, env)

	require.Len(t, entries, 1)
	assert.Equal(t, "contact", entries[0].TargetType)
	assert.Equal(t, publicid.Encode(fixtures.ContactExportSubjectID), entries[0].TargetID)
	assert.Equal(t, "John", entries[0].Actor.Name)
}
