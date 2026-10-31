package site_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func revenueSettings(t *testing.T, c *siteapi.Client) *siteapi.SiteRevenueSettingsResource {
	t.Helper()
	res, err := c.SiteWorkspacesGetRevenueSettings(context.Background(),
		siteapi.SiteWorkspacesGetRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteRevenueSettingsResource)
	require.Truef(t, ok, "got %T", res)
	return got
}

func updateRevenueSettings(t *testing.T, c *siteapi.Client, in siteapi.SiteUpdateRevenueSettingsInput) siteapi.SiteWorkspacesUpdateRevenueSettingsRes {
	t.Helper()
	res, err := c.SiteWorkspacesUpdateRevenueSettings(context.Background(), &in,
		siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return res
}

func TestTheReportingCurrencyIsUnsetAndTheWindowIsFiveDaysUntilChosen(t *testing.T) {
	env := testhelper.Setup(t)

	got := revenueSettings(t, env.SiteActor(t, fixtures.MemberMaryEmail))

	assert.True(t, got.ReportingCurrency.Null)
	assert.Equal(t, siteapi.AttributionWindowDays(5), got.AttributionWindowDays)
}

func TestAnAdminChoosesTheCurrencyAndTheWindowAndEveryMemberReadsThem(t *testing.T) {
	env := testhelper.Setup(t)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	res := updateRevenueSettings(t, admin, siteapi.SiteUpdateRevenueSettingsInput{
		ReportingCurrency:     siteapi.NewOptCurrencyCode("EUR"),
		AttributionWindowDays: siteapi.NewOptAttributionWindowDays(14),
	})
	updated, ok := res.(*siteapi.SiteRevenueSettingsResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.CurrencyCode("EUR"), updated.ReportingCurrency.Value)
	assert.Equal(t, siteapi.AttributionWindowDays(14), updated.AttributionWindowDays)

	got := revenueSettings(t, env.SiteActor(t, fixtures.MemberMaryEmail))
	assert.Equal(t, siteapi.CurrencyCode("EUR"), got.ReportingCurrency.Value)
	assert.Equal(t, siteapi.AttributionWindowDays(14), got.AttributionWindowDays)
}

func TestAFieldLeftOutOfTheUpdateStaysAsItWas(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	updateRevenueSettings(t, owner, siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("JPY")})

	updateRevenueSettings(t, owner, siteapi.SiteUpdateRevenueSettingsInput{AttributionWindowDays: siteapi.NewOptAttributionWindowDays(7)})

	got := revenueSettings(t, owner)
	assert.Equal(t, siteapi.CurrencyCode("JPY"), got.ReportingCurrency.Value)
	assert.Equal(t, siteapi.AttributionWindowDays(7), got.AttributionWindowDays)
}

func TestAMemberCannotChangeTheRevenueSettings(t *testing.T) {
	env := testhelper.Setup(t)

	member := updateRevenueSettings(t, env.SiteActor(t, fixtures.MemberMaryEmail),
		siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("USD")})
	outsider := updateRevenueSettings(t, env.SiteActor(t, fixtures.OutsiderOscarEmail),
		siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("USD")})

	assert.IsType(t, &siteapi.SiteWorkspacesUpdateRevenueSettingsForbidden{}, member)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateRevenueSettingsNotFound{}, outsider)
	assert.True(t, revenueSettings(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).ReportingCurrency.Null)
	assert.Empty(t, env.OutboxEvents(t, events.NameAuditEntry))
}

func TestAnUnknownCurrencyCodeIsRefusedWithAReasonAndNothingChanges(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := updateRevenueSettings(t, owner, siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("QQQ")})

	refused, ok := res.(*siteapi.SiteWorkspacesUpdateRevenueSettingsUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Errors.Value["reportingCurrency"][0], "unknown currency")
	assert.True(t, revenueSettings(t, owner).ReportingCurrency.Null)
	assert.Empty(t, env.OutboxEvents(t, events.NameAuditEntry))
}

func TestAWindowOutsideItsBoundsIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	for _, days := range []siteapi.AttributionWindowDays{0, 91} {
		res, err := owner.SiteWorkspacesUpdateRevenueSettings(context.Background(),
			&siteapi.SiteUpdateRevenueSettingsInput{AttributionWindowDays: siteapi.NewOptAttributionWindowDays(days)},
			siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteWorkspacesUpdateRevenueSettingsBadRequest{}, res, "days=%d", days)
	}

	assert.Equal(t, siteapi.AttributionWindowDays(5), revenueSettings(t, owner).AttributionWindowDays)
	assert.Empty(t, env.OutboxEvents(t, events.NameAuditEntry))
}

func TestEveryChangeOfTheRevenueSettingsIsAuditedAndARepeatIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	updateRevenueSettings(t, admin, siteapi.SiteUpdateRevenueSettingsInput{
		ReportingCurrency:     siteapi.NewOptCurrencyCode("EUR"),
		AttributionWindowDays: siteapi.NewOptAttributionWindowDays(10),
	})
	updateRevenueSettings(t, admin, siteapi.SiteUpdateRevenueSettingsInput{
		ReportingCurrency:     siteapi.NewOptCurrencyCode("EUR"),
		AttributionWindowDays: siteapi.NewOptAttributionWindowDays(10),
	})

	published := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, published, 1)
	entry := published[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionWorkspaceUpdate, entry.Action)
	diff, err := json.Marshal(entry.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"reporting_currency":{"from":null,"to":"EUR"},"attribution_window_days":{"from":5,"to":10}}`, string(diff))

	recorded := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceUpdate)
	require.Len(t, recorded, 1)
	assert.Equal(t, fixtures.AdminAliceName, recorded[0].Actor.Name.Value)
}
