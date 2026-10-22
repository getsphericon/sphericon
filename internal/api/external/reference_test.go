package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalCustomFieldsList(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	denied, err := env.ExternalScoped(t, "contacts:read").CustomFieldsList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ProblemDetails{}, denied, "custom_fields:read is required")

	c := env.ExternalScoped(t, "custom_fields:read")
	res, err := c.CustomFieldsList(ctx)
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.CustomFieldsListOK)
	require.Truef(t, isOK, "got %T", res)

	types := map[string]externalapi.CustomFieldType{}
	for _, f := range ok.Items {
		types[f.Key] = f.Type
	}
	assert.Equal(t, externalapi.CustomFieldType("string"), types["plan"])
	assert.Equal(t, externalapi.CustomFieldType("number"), types["courses_completed"])
	assert.Equal(t, externalapi.CustomFieldType("bool"), types["is_trial"])
}

func TestExternalReferenceDataIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// The Globex tenant has its own field and domain; the acme token must not see them.
	c := env.ExternalScoped(t, "custom_fields:read", "sending_domains:read")

	fields, err := c.CustomFieldsList(ctx)
	require.NoError(t, err)
	for _, f := range fields.(*externalapi.CustomFieldsListOK).Items {
		assert.NotEqual(t, fixtures.CustomFieldGlobexKey, f.Key)
	}

	domains, err := c.SendingDomainsList(ctx, externalapi.SendingDomainsListParams{})
	require.NoError(t, err)
	for _, d := range domains.(*externalapi.SendingDomainsListOK).Items {
		assert.NotEqual(t, fixtures.SendingDomainGlobexDomain, d.Domain)
	}

	rates, err := c.SendingDomainRatesList(ctx, externalapi.SendingDomainRatesListParams{})
	require.NoError(t, err)
	for _, r := range rates.(*externalapi.SendingDomainRatesListOK).Items {
		assert.NotEqual(t, fixtures.SendingDomainGlobexDomain, r.Domain)
	}
}

func TestExternalSendingDomainsList(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	denied, err := env.ExternalScoped(t, "contacts:read").
		SendingDomainsList(ctx, externalapi.SendingDomainsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsListUnauthorized{}, denied)

	c := env.ExternalScoped(t, "sending_domains:read")
	res, err := c.SendingDomainsList(ctx, externalapi.SendingDomainsListParams{})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.SendingDomainsListOK)
	require.Truef(t, isOK, "got %T", res)

	verified := map[string]bool{}
	for _, d := range ok.Items {
		verified[d.Domain] = d.Verified
	}
	assert.Equal(t, map[string]bool{"mail.acme.com": true, "news.acme.com": false, "codebasics.dev": true, "launch.acme.com": false, "lost.acme.com": false}, verified)
}

func ratesByDomain(t *testing.T, res externalapi.SendingDomainRatesListRes) map[string]externalapi.SendingDomainRatesResource {
	t.Helper()
	ok, isOK := res.(*externalapi.SendingDomainRatesListOK)
	require.Truef(t, isOK, "got %T", res)
	out := map[string]externalapi.SendingDomainRatesResource{}
	for _, r := range ok.Items {
		out[r.Domain] = r
	}
	return out
}

// The rates are the (numerator, denominator, rate) triple of ADR 0011: complaints over
// delivered (ADR 0053), hard bounces over sent, by each Event's own timestamp.
func TestExternalSendingDomainRates(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	denied, err := env.ExternalScoped(t, "contacts:read").
		SendingDomainRatesList(ctx, externalapi.SendingDomainRatesListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainRatesListUnauthorized{}, denied)

	c := env.ExternalScoped(t, "sending_domains:read")

	// Default 7-day window: mail.acme.com has 8 sent, 1 hard bounce (the soft one is
	// excluded), 6 delivered, 1 complaint; the 40-day-old events fall outside.
	res, err := c.SendingDomainRatesList(ctx, externalapi.SendingDomainRatesListParams{})
	require.NoError(t, err)
	rates := ratesByDomain(t, res)

	mail := rates["mail.acme.com"]
	assert.EqualValues(t, 7, mail.WindowDays)
	assert.EqualValues(t, 1, mail.ComplaintRate.Numerator)
	assert.EqualValues(t, 6, mail.ComplaintRate.Denominator)
	assert.InDelta(t, 1.0/6.0, mail.ComplaintRate.Rate.Value, 1e-9)
	assert.EqualValues(t, 1, mail.BounceRate.Numerator)
	assert.EqualValues(t, 8, mail.BounceRate.Denominator)
	assert.InDelta(t, 1.0/8.0, mail.BounceRate.Rate.Value, 1e-9)

	clean := rates["codebasics.dev"]
	assert.EqualValues(t, 0, clean.ComplaintRate.Numerator)
	assert.EqualValues(t, 2, clean.ComplaintRate.Denominator)
	assert.False(t, clean.ComplaintRate.Rate.Null)
	assert.Zero(t, clean.ComplaintRate.Rate.Value)

	// No traffic: the rate is undefined (null), never reported as zero.
	idle := rates["news.acme.com"]
	assert.EqualValues(t, 0, idle.ComplaintRate.Denominator)
	assert.True(t, idle.ComplaintRate.Rate.Null)
	assert.True(t, idle.BounceRate.Rate.Null)

	// A 90-day window pulls in the old send and complaint: 2/7 (delivered).
	res, err = c.SendingDomainRatesList(ctx, externalapi.SendingDomainRatesListParams{
		WindowDays: externalapi.NewOptInt32(90),
	})
	require.NoError(t, err)
	wide := ratesByDomain(t, res)["mail.acme.com"]
	assert.EqualValues(t, 90, wide.WindowDays)
	assert.EqualValues(t, 2, wide.ComplaintRate.Numerator)
	assert.EqualValues(t, 7, wide.ComplaintRate.Denominator)
}
