package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func actions(items []siteapi.SiteAuditEntryResource) []string {
	out := make([]string, len(items))
	for i := range items {
		out[i] = string(items[i].Action)
	}
	return out
}

func mustExport(ctx context.Context, t *testing.T, c *siteapi.Client, p siteapi.SiteAuditExportParams) siteapi.SiteAuditExportRes {
	t.Helper()
	res, err := c.SiteAuditExport(ctx, p)
	require.NoError(t, err)
	return res
}

// The filter tests run on Globex's fixture log: its base entry (user 2, membership.update,
// January) plus the four fixture entries that differ in every filterable column.
func TestAuditFiltersNarrowListAndExportAlike(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJaneEmail)
	ctx := context.Background()
	day := func(m time.Month) siteapi.OptTimestamp {
		return siteapi.NewOptTimestamp(siteapi.Timestamp(time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)))
	}
	str := siteapi.NewOptString
	system := siteapi.NewOptSiteAuditActorKind(siteapi.SiteAuditActorKindSystem)

	cases := []struct {
		name   string
		list   siteapi.SiteAuditListParams
		export siteapi.SiteAuditExportParams
		want   []string
	}{
		{"actor kind",
			siteapi.SiteAuditListParams{ActorKind: system},
			siteapi.SiteAuditExportParams{ActorKind: system},
			[]string{"webhook_endpoint.create"}},
		{"actor id",
			siteapi.SiteAuditListParams{ActorId: str("77")},
			siteapi.SiteAuditExportParams{ActorId: str("77")},
			[]string{"integration.update"}},
		{"action",
			siteapi.SiteAuditListParams{Action: siteapi.NewOptAuditAction(siteapi.AuditActionIntegrationDelete)},
			siteapi.SiteAuditExportParams{Action: siteapi.NewOptAuditAction(siteapi.AuditActionIntegrationDelete)},
			[]string{"integration.delete"}},
		{"target type",
			siteapi.SiteAuditListParams{TargetType: str("integration")},
			siteapi.SiteAuditExportParams{TargetType: str("integration")},
			[]string{"integration.delete", "integration.update"}},
		{"target type and id",
			siteapi.SiteAuditListParams{TargetType: str("integration"), TargetId: str("5")},
			siteapi.SiteAuditExportParams{TargetType: str("integration"), TargetId: str("5")},
			[]string{"integration.update"}},
		{"ip",
			siteapi.SiteAuditListParams{IP: str("10.0.0.1")},
			siteapi.SiteAuditExportParams{IP: str("10.0.0.1")},
			[]string{"integration.delete", "integration.update"}},
		{"request id",
			siteapi.SiteAuditListParams{RequestId: str("req-b")},
			siteapi.SiteAuditExportParams{RequestId: str("req-b")},
			[]string{"webhook_endpoint.create"}},
		{"period",
			siteapi.SiteAuditListParams{From: day(time.February), To: day(time.April)},
			siteapi.SiteAuditExportParams{From: day(time.February), To: day(time.April)},
			[]string{"webhook_endpoint.create", "integration.update"}},
		{"filters combine",
			siteapi.SiteAuditListParams{TargetType: str("integration"), From: day(time.March)},
			siteapi.SiteAuditExportParams{TargetType: str("integration"), From: day(time.March)},
			[]string{"integration.delete"}},
		{"no match",
			siteapi.SiteAuditListParams{Action: siteapi.NewOptAuditAction(siteapi.AuditActionRuleDelete)},
			siteapi.SiteAuditExportParams{Action: siteapi.NewOptAuditAction(siteapi.AuditActionRuleDelete)},
			[]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.list.WorkspaceId, tc.export.WorkspaceId = fixtures.GlobexPublicID, fixtures.GlobexPublicID
			res, err := owner.SiteAuditList(ctx, tc.list)
			require.NoError(t, err)
			page, ok := res.(*siteapi.SiteAuditEntryList)
			require.True(t, ok)
			assert.Equal(t, tc.want, actions(page.Items))

			rows := auditCSV(t, mustExport(ctx, t, owner, tc.export))
			assert.Len(t, rows, 1+len(tc.want), "the export matches the filter")
		})
	}
}

func TestAuditFilteredCursorPaginates(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJaneEmail)
	ctx := context.Background()
	params := siteapi.SiteAuditListParams{
		WorkspaceId: fixtures.GlobexPublicID, TargetType: siteapi.NewOptString("integration"), Limit: siteapi.NewOptInt32(1),
	}
	res, err := owner.SiteAuditList(ctx, params)
	require.NoError(t, err)
	p1 := res.(*siteapi.SiteAuditEntryList)
	require.Equal(t, []string{"integration.delete"}, actions(p1.Items))
	require.True(t, p1.NextCursor.IsSet())

	params.Cursor = siteapi.NewOptString(p1.NextCursor.Value)
	res, err = owner.SiteAuditList(ctx, params)
	require.NoError(t, err)
	p2 := res.(*siteapi.SiteAuditEntryList)
	assert.Equal(t, []string{"integration.update"}, actions(p2.Items))
	assert.Empty(t, p2.NextCursor.Value)
}
