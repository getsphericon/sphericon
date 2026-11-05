package site_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	got, err := c.SiteWorkspacesGetRevenueSettings(context.Background(),
		siteapi.SiteWorkspacesGetRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return got
}

func updateRevenueSettings(t *testing.T, c *siteapi.Client, in siteapi.SiteUpdateRevenueSettingsInput) (*siteapi.SiteRevenueSettingsResource, error) {
	t.Helper()
	return c.SiteWorkspacesUpdateRevenueSettings(context.Background(), &in,
		siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
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

	updated, err := updateRevenueSettings(t, admin, siteapi.SiteUpdateRevenueSettingsInput{
		ReportingCurrency:     siteapi.NewOptCurrencyCode("EUR"),
		AttributionWindowDays: siteapi.NewOptAttributionWindowDays(14),
	})
	require.NoError(t, err)
	assert.Equal(t, siteapi.CurrencyCode("EUR"), updated.ReportingCurrency.Value)
	assert.Equal(t, siteapi.AttributionWindowDays(14), updated.AttributionWindowDays)

	got := revenueSettings(t, env.SiteActor(t, fixtures.MemberMaryEmail))
	assert.Equal(t, siteapi.CurrencyCode("EUR"), got.ReportingCurrency.Value)
	assert.Equal(t, siteapi.AttributionWindowDays(14), got.AttributionWindowDays)
}

func TestAFieldLeftOutOfTheUpdateStaysAsItWas(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := updateRevenueSettings(t, owner, siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("JPY")})
	require.NoError(t, err)

	_, err = updateRevenueSettings(t, owner, siteapi.SiteUpdateRevenueSettingsInput{AttributionWindowDays: siteapi.NewOptAttributionWindowDays(7)})
	require.NoError(t, err)

	got := revenueSettings(t, owner)
	assert.Equal(t, siteapi.CurrencyCode("JPY"), got.ReportingCurrency.Value)
	assert.Equal(t, siteapi.AttributionWindowDays(7), got.AttributionWindowDays)
}

func TestAMemberCannotChangeTheRevenueSettings(t *testing.T) {
	env := testhelper.Setup(t)

	_, member := updateRevenueSettings(t, env.SiteActor(t, fixtures.MemberMaryEmail),
		siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("USD")})
	_, outsider := updateRevenueSettings(t, env.SiteActor(t, fixtures.OutsiderOscarEmail),
		siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("USD")})

	testhelper.SiteRefusal(t, member, http.StatusForbidden)
	testhelper.SiteRefusal(t, outsider, http.StatusNotFound)
	assert.True(t, revenueSettings(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).ReportingCurrency.Null)
	assert.Empty(t, env.QueuedEvents(t, events.NameAuditEntry))
}

func TestAnUnknownCurrencyCodeIsRefusedWithAReasonAndNothingChanges(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := updateRevenueSettings(t, owner, siteapi.SiteUpdateRevenueSettingsInput{ReportingCurrency: siteapi.NewOptCurrencyCode("QQQ")})

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Errors.Value["reportingCurrency"][0], "unknown currency")
	assert.True(t, revenueSettings(t, owner).ReportingCurrency.Null)
	assert.Empty(t, env.QueuedEvents(t, events.NameAuditEntry))
}

func TestAWindowOutsideItsBoundsIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	for _, days := range []siteapi.AttributionWindowDays{0, 91} {
		t.Run(fmt.Sprintf("days=%d", days), func(t *testing.T) {
			_, err := owner.SiteWorkspacesUpdateRevenueSettings(context.Background(),
				&siteapi.SiteUpdateRevenueSettingsInput{AttributionWindowDays: siteapi.NewOptAttributionWindowDays(days)},
				siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: fixtures.AcmePublicID})
			testhelper.SiteRefusal(t, err, http.StatusBadRequest)
		})
	}

	assert.Equal(t, siteapi.AttributionWindowDays(5), revenueSettings(t, owner).AttributionWindowDays)
	assert.Empty(t, env.QueuedEvents(t, events.NameAuditEntry))
}

func TestEveryChangeOfTheRevenueSettingsIsAuditedAndARepeatIsNot(t *testing.T) {
	env := testhelper.Setup(t)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	_, err := updateRevenueSettings(t, admin, siteapi.SiteUpdateRevenueSettingsInput{
		ReportingCurrency:     siteapi.NewOptCurrencyCode("EUR"),
		AttributionWindowDays: siteapi.NewOptAttributionWindowDays(10),
	})
	require.NoError(t, err)
	_, err = updateRevenueSettings(t, admin, siteapi.SiteUpdateRevenueSettingsInput{
		ReportingCurrency:     siteapi.NewOptCurrencyCode("EUR"),
		AttributionWindowDays: siteapi.NewOptAttributionWindowDays(10),
	})
	require.NoError(t, err)

	published := env.QueuedEvents(t, events.NameAuditEntry)
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

func TestTheFirstOrderCompletedChoosesTheReportingCurrencyAsTheSystemOnce(t *testing.T) {
	env := testhelper.Setup(t)
	s := newShop(t, env)

	s.completed("A-1", "10.00", "EUR")
	env.WorkOrderJobs(t)
	s.completed("A-2", "10.00", "USD")
	env.WorkOrderJobs(t)
	env.WorkOrderJobs(t) // river retries a job: every orders job again

	assert.Equal(t, siteapi.CurrencyCode("EUR"), revenueSettings(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).ReportingCurrency.Value)
	published := env.QueuedEvents(t, events.NameAuditEntry)
	require.Len(t, published, 1)
	entry := published[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionWorkspaceUpdate, entry.Action)
	assert.Equal(t, events.ActorSystem, entry.Actor.Kind)
	diff, err := json.Marshal(entry.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"reporting_currency":{"to":"EUR"}}`, string(diff))
}
