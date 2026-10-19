package site_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A new Sending domain lists the Tracking domain CNAME (ADR 0038): track.<domain> at the
// configured edge host, pending and never checked.
func TestSiteSendingDomainShowsTheTrackingRecordPending(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteSendingDomainsCreate(context.Background(), &siteapi.SiteCreateSendingDomainInput{Domain: "marketing.example.com"},
		siteapi.SiteSendingDomainsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", res)

	rec, ok := created.TrackingRecord.Get()
	require.True(t, ok, "the tracking record is listed")
	assert.Equal(t, siteapi.SiteTrackingRecordTypeCNAME, rec.Type)
	assert.Equal(t, "track.marketing.example.com", rec.Host)
	assert.Equal(t, testhelper.TrackingCNAMETarget, rec.Target)
	assert.Equal(t, siteapi.SiteTrackingStatePending, rec.State)
	assert.False(t, rec.LastCheckedAt.IsSet(), "never checked")
	assert.False(t, rec.StateChangedAt.IsSet(), "never changed")
}

// The state and its two times are read back as stored.
func TestSiteSendingDomainShowsTheTrackingState(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	checked := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	changed := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).SendingDomain().UpdateOneID(fixtures.SendingDomainVerifiedID).
		SetTrackingState(sendingdomain.TrackingStateServing).
		SetTrackingCheckedAt(checked).
		SetTrackingStateChangedAt(changed).Exec(ctx))

	res, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{Slug: fixtures.AcmeSlug, ID: fixtures.SendingDomainVerifiedID})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", res)

	rec, ok := got.TrackingRecord.Get()
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteTrackingStateServing, rec.State)
	assert.True(t, checked.Equal(time.Time(rec.LastCheckedAt.Value)))
	assert.True(t, changed.Equal(time.Time(rec.StateChangedAt.Value)))
}

// With SPHERICON_TRACKING_CNAME_TARGET unset the feature is off: no tracking record.
func TestSiteSendingDomainHasNoTrackingRecordWithoutATarget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackingCNAMETarget = "" }))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteSendingDomainsGet(context.Background(), siteapi.SiteSendingDomainsGetParams{Slug: fixtures.AcmeSlug, ID: fixtures.SendingDomainVerifiedID})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", res)

	assert.False(t, got.TrackingRecord.IsSet())
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "trackingRecord")
}
