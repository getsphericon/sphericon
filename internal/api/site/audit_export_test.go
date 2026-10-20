package site_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func auditExport(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) siteapi.SiteAuditExportRes {
	t.Helper()
	res, err := c.SiteAuditExport(context.Background(), siteapi.SiteAuditExportParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func auditCSV(t *testing.T, res siteapi.SiteAuditExportRes) [][]string {
	t.Helper()
	ok, isOK := res.(*siteapi.SiteAuditExportOKHeaders)
	require.Truef(t, isOK, "export answered %T", res)
	assert.Contains(t, ok.ContentDisposition, "attachment")
	assert.Contains(t, ok.ContentDisposition, ".csv")
	body, err := io.ReadAll(ok.Response.Data)
	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	require.NoError(t, err)
	return rows
}

func TestAuditExportIsTheWholeLogAsCSV(t *testing.T) {
	env := testhelper.Setup(t)
	promoteMary(t, env)

	rows := auditCSV(t, auditExport(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID))
	require.Len(t, rows, 3, "header, the new entry, the fixture entry")
	header := rows[0]
	assert.Equal(t, []string{
		"id", "occurred_at", "actor_kind", "actor_id", "actor_name", "action",
		"target_type", "target_id", "target_name", "diff", "request_id", "ip", "user_agent",
	}, header)

	col := func(row []string, name string) string {
		for i, h := range header {
			if h == name {
				return row[i]
			}
		}
		t.Fatalf("no column %q", name)
		return ""
	}
	assert.Equal(t, "membership.update", col(rows[1], "action"), "newest first")
	assert.Equal(t, "John", col(rows[1], "actor_name"))
	assert.JSONEq(t, `{"role":{"from":"member","to":"admin"}}`, col(rows[1], "diff"))
	assert.JSONEq(t, `{"role":{"from":"admin","to":"member"}}`, col(rows[2], "diff"))
}

func TestAuditExportSpansPagesAndNeverLeaksAnotherTenant(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	const extra = 250 // more than one internal page
	for i := range extra {
		require.NoError(t, s.AuditEntry().Create().
			SetEntryKey("bulk-"+strconv.Itoa(i)).SetOccurredAt(time.Now()).
			SetActorKind("system").SetAction("tag.create").SetTargetType("tag").
			Exec(ctx))
	}

	rows := auditCSV(t, auditExport(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID))
	assert.Len(t, rows, 1+extra+1, "header, every extra entry and the fixture entry; none of Globex's")
}

func TestAuditExportNeutralisesSpreadsheetFormulas(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	rows := auditCSV(t, mustExport(ctx, t, env.SiteActor(t, fixtures.OwnerJaneEmail), siteapi.SiteAuditExportParams{
		WorkspaceId: fixtures.GlobexPublicID, Action: siteapi.NewOptString(fixtures.GlobexAuditFormulaEntryAction)}))
	require.Len(t, rows, 2)
	for _, cell := range rows[1] {
		assert.False(t, strings.ContainsAny(cell[:min(1, len(cell))], "=+-@\t\r"), "cell %q starts a formula", cell)
	}
	assert.Contains(t, rows[1], "'=HYPERLINK(\"http://evil\")")
	assert.Contains(t, rows[1], "'+cmd")
}

func TestAuditExportIsOwnerAndAdminOnlyAndLicensed(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	assert.IsType(t, &siteapi.SiteAuditExportForbidden{}, auditExport(t, mary, fixtures.AcmePublicID), "a member is refused")

	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	assert.IsType(t, &siteapi.SiteAuditExportNotFound{}, auditExport(t, oscar, fixtures.AcmePublicID), "a stranger finds no workspace")

	promoteMary(t, env)
	assert.IsType(t, &siteapi.SiteAuditExportOKHeaders{}, auditExport(t, mary, fixtures.AcmePublicID), "an admin exports")
}

func TestAuditExportWithoutLicenseIsPaymentRequired(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	assert.IsType(t, &siteapi.SiteAuditExportPaymentRequired{}, auditExport(t, owner, fixtures.AcmePublicID))
}

// The export is itself an audit_log.export entry that names the filter it ran with
// (story 19); the entry is recorded for the caller, in the Workspace's own log.
func TestAuditExportIsItselfAudited(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	auditCSV(t, mustExport(ctx, t, owner, siteapi.SiteAuditExportParams{WorkspaceId: fixtures.AcmePublicID, Action: siteapi.NewOptString("membership.update")}))

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionAuditLogExport)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, "John", got[0].Actor.Name.Value)
	assert.Equal(t, "audit_log", got[0].Target.Type)
	diff, err := json.Marshal(got[0].Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"filter":{"action":"membership.update"}}`, string(diff))
}
