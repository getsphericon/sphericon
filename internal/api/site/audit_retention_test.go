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

func setRetention(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, days siteapi.NilRetentionDays) siteapi.SiteAuditSetRetentionRes {
	t.Helper()
	res, err := c.SiteAuditSetRetention(context.Background(), &siteapi.SiteAuditRetention{RetentionDays: days},
		siteapi.SiteAuditSetRetentionParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func retentionOf(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) siteapi.SiteAuditGetRetentionRes {
	t.Helper()
	res, err := c.SiteAuditGetRetention(context.Background(), siteapi.SiteAuditGetRetentionParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func TestAuditRetentionIsSetReadAndRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	first := retentionOf(t, owner, fixtures.AcmePublicID)
	require.IsType(t, &siteapi.SiteAuditRetention{}, first)
	assert.True(t, first.(*siteapi.SiteAuditRetention).RetentionDays.Null, "no window by default")

	res := setRetention(t, owner, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(90))
	require.IsType(t, &siteapi.SiteAuditRetention{}, res)
	got := retentionOf(t, owner, fixtures.AcmePublicID).(*siteapi.SiteAuditRetention)
	assert.Equal(t, siteapi.RetentionDays(90), got.RetentionDays.Value)

	got2 := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, got2, 1)
	e := got2[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionWorkspaceUpdate, e.Action)
	diff, err := json.Marshal(e.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"retention_days":{"from":null,"to":90}}`, string(diff))

	// Setting the same value again records nothing; clearing records the change back.
	setRetention(t, owner, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(90))
	assert.Len(t, env.OutboxEvents(t, events.NameAuditEntry), 1)
	setRetention(t, owner, fixtures.AcmePublicID, siteapi.NilRetentionDays{Null: true})
	assert.Len(t, env.OutboxEvents(t, events.NameAuditEntry), 2)
	assert.True(t, retentionOf(t, owner, fixtures.AcmePublicID).(*siteapi.SiteAuditRetention).RetentionDays.Null)
}

func TestAuditRetentionIsOwnerAdminOnlyLicensed(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	assert.IsType(t, &siteapi.SiteAuditSetRetentionForbidden{}, setRetention(t, mary, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(30)))
	assert.IsType(t, &siteapi.SiteAuditGetRetentionForbidden{}, retentionOf(t, mary, fixtures.AcmePublicID))
	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	assert.IsType(t, &siteapi.SiteAuditSetRetentionNotFound{}, setRetention(t, oscar, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(30)))
	assert.IsType(t, &siteapi.SiteAuditGetRetentionNotFound{}, retentionOf(t, oscar, fixtures.AcmePublicID))

	unlicensed := testhelper.Setup(t, testhelper.WithoutLicense()).SiteActor(t, fixtures.OwnerJohnEmail)
	assert.IsType(t, &siteapi.SiteAuditSetRetentionPaymentRequired{}, setRetention(t, unlicensed, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(30)))
	assert.IsType(t, &siteapi.SiteAuditGetRetentionPaymentRequired{}, retentionOf(t, unlicensed, fixtures.AcmePublicID))
}
