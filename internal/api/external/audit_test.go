package external_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/mokevnin/sphericon/gen/external"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

func TestExternalAuditListNeedsTheAuditReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	for _, scopes := range [][]string{{}, {"contacts:read", "events:read", "tokens:read"}} {
		c := env.ExternalScoped(t, scopes...)
		res, err := c.AuditEntriesList(ctx, externalapi.AuditEntriesListParams{})
		require.NoError(t, err)
		assert.IsType(t, &externalapi.AuditEntriesListUnauthorized{}, res, "scopes %v are refused", scopes)
	}

	res, err := env.ExternalScoped(t, "audit:read").AuditEntriesList(ctx, externalapi.AuditEntriesListParams{})
	require.NoError(t, err)
	page, ok := res.(*externalapi.AuditEntryList)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, page.Items, 1, "only the Workspace's own fixture entry")
	e := page.Items[0]
	assert.Equal(t, strconv.Itoa(fixtures.AcmeAuditEntryID), string(e.ID))
	assert.Equal(t, fixtures.AcmeAuditEntryAction, e.Action)
	assert.Equal(t, externalapi.AuditActorKindUser, e.Actor.Kind)
	diff, err := json.Marshal(e.Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":{"from":"admin","to":"member"}}`, string(diff))
}

func TestExternalAuditListIsTenantIsolated(t *testing.T) {
	env := testhelper.Setup(t)
	globex := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, "audit:read"))
	res, err := globex.AuditEntriesList(context.Background(), externalapi.AuditEntriesListParams{})
	require.NoError(t, err)
	page := res.(*externalapi.AuditEntryList)
	require.Len(t, page.Items, 5, "Globex's own fixture entries, none of Acme's")
	assert.Equal(t, strconv.Itoa(fixtures.GlobexAuditEntryID), string(page.Items[len(page.Items)-1].ID), "oldest last")
}

func TestExternalAuditListPaginatesByCursor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, "audit:read"))

	var seen []string
	params := externalapi.AuditEntriesListParams{Limit: externalapi.NewOptInt32(2)}
	for range 3 {
		res, err := c.AuditEntriesList(ctx, params)
		require.NoError(t, err)
		page := res.(*externalapi.AuditEntryList)
		for _, e := range page.Items {
			seen = append(seen, string(e.ID))
		}
		if page.NextCursor.Value == "" {
			break
		}
		params.Cursor = externalapi.NewOptString(page.NextCursor.Value)
	}
	assert.Equal(t, []string{"6", "5", "4", "3", "1"}, seen, "newest first across three pages, the last one short")
}

func TestExternalAuditFiltersNarrowTheRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, "audit:read"))
	str := externalapi.NewOptString
	day := func(m time.Month) externalapi.OptTimestamp {
		return externalapi.NewOptTimestamp(externalapi.Timestamp(time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)))
	}

	cases := []struct {
		name   string
		params externalapi.AuditEntriesListParams
		want   []string
	}{
		{"actor kind", externalapi.AuditEntriesListParams{ActorKind: externalapi.NewOptAuditActorKind(externalapi.AuditActorKindSystem)}, []string{fixtures.GlobexAuditWebhookCreateAction}},
		{"actor id", externalapi.AuditEntriesListParams{ActorId: str("77")}, []string{fixtures.GlobexAuditIntegrationUpdateAction}},
		{"action", externalapi.AuditEntriesListParams{Action: str(fixtures.GlobexAuditIntegrationDeleteAction)}, []string{fixtures.GlobexAuditIntegrationDeleteAction}},
		{"target type and id", externalapi.AuditEntriesListParams{TargetType: str("integration"), TargetId: str("5")}, []string{fixtures.GlobexAuditIntegrationUpdateAction}},
		{"ip", externalapi.AuditEntriesListParams{IP: str("10.0.0.1")}, []string{fixtures.GlobexAuditIntegrationDeleteAction, fixtures.GlobexAuditIntegrationUpdateAction}},
		{"request id", externalapi.AuditEntriesListParams{RequestId: str("req-b")}, []string{fixtures.GlobexAuditWebhookCreateAction}},
		{"period", externalapi.AuditEntriesListParams{From: day(time.February), To: day(time.April)}, []string{fixtures.GlobexAuditWebhookCreateAction, fixtures.GlobexAuditIntegrationUpdateAction}},
		{"no match", externalapi.AuditEntriesListParams{Action: str("nothing.here")}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := c.AuditEntriesList(context.Background(), tc.params)
			require.NoError(t, err)
			page := res.(*externalapi.AuditEntryList)
			got := make([]string, len(page.Items))
			for i, e := range page.Items {
				got[i] = e.Action
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// An Operator's id cannot be probed through /api either.
func TestExternalAuditOperatorIdCannotBeProbed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.Operator("op-42"), "abuse")
	require.NoError(t, err)
	env.DeliverToEE(t)
	c := env.ExternalScoped(t, "audit:read")

	count := func(p externalapi.AuditEntriesListParams) int {
		res, err := c.AuditEntriesList(ctx, p)
		require.NoError(t, err)
		return len(res.(*externalapi.AuditEntryList).Items)
	}
	assert.Zero(t, count(externalapi.AuditEntriesListParams{ActorId: externalapi.NewOptString("op-42")}))
	assert.Equal(t, 1, count(externalapi.AuditEntriesListParams{
		ActorKind: externalapi.NewOptAuditActorKind(externalapi.AuditActorKindOperator), ActorId: externalapi.NewOptString("anything")}))
}

func TestExternalAuditListWithoutLicenseIsPaymentRequired(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	res, err := env.ExternalScoped(t, "audit:read").AuditEntriesList(context.Background(), externalapi.AuditEntriesListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuditEntriesListPaymentRequired{}, res)
}

func TestExternalAuditListRejectsABadCursor(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.ExternalScoped(t, "audit:read").AuditEntriesList(context.Background(),
		externalapi.AuditEntriesListParams{Cursor: externalapi.NewOptString("nope")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuditEntriesListBadRequest{}, res)
}
