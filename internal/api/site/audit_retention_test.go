package site_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setRetention(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, days siteapi.NilRetentionDays) error {
	t.Helper()
	// A change names the revision it was made on (ADR 0119), so the client reads the Workspace first.
	read, err := c.SiteWorkspacesGet(context.Background(), siteapi.SiteWorkspacesGetParams{WorkspaceId: workspaceID})
	if err != nil {
		return err
	}
	_, err = c.SiteAuditSetRetention(context.Background(), &siteapi.SiteAuditRetention{Revision: read.Revision, RetentionDays: days},
		siteapi.SiteAuditSetRetentionParams{WorkspaceId: workspaceID})
	return err
}

func retentionOf(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) (*siteapi.SiteAuditRetention, error) {
	t.Helper()
	return c.SiteAuditGetRetention(context.Background(), siteapi.SiteAuditGetRetentionParams{WorkspaceId: workspaceID})
}

func TestAuditRetentionIsSetReadAndRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	first, err := retentionOf(t, owner, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.True(t, first.RetentionDays.Null, "no window by default")

	err = setRetention(t, owner, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(90))
	require.NoError(t, err)
	got, err := retentionOf(t, owner, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.Equal(t, siteapi.RetentionDays(90), got.RetentionDays.Value)

	got2 := env.QueuedEvents(t, events.NameAuditEntry)
	require.Len(t, got2, 1)
	e := got2[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionWorkspaceUpdate, e.Action)
	diff, err := json.Marshal(e.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"retention_days":{"from":null,"to":90}}`, string(diff))

	// Setting the same value again records nothing; clearing records the change back.
	err = setRetention(t, owner, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(90))
	require.NoError(t, err)
	assert.Len(t, env.QueuedEvents(t, events.NameAuditEntry), 1)
	err = setRetention(t, owner, fixtures.AcmePublicID, siteapi.NilRetentionDays{Null: true})
	require.NoError(t, err)
	assert.Len(t, env.QueuedEvents(t, events.NameAuditEntry), 2)
	cleared, err := retentionOf(t, owner, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.True(t, cleared.RetentionDays.Null)
}

func TestAuditRetentionIsOwnerAdminOnlyLicensed(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	err := setRetention(t, mary, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(30))
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	_, err = retentionOf(t, mary, fixtures.AcmePublicID)
	testhelper.SiteRefusal(t, err, http.StatusForbidden)
	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	err = setRetention(t, oscar, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(30))
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = retentionOf(t, oscar, fixtures.AcmePublicID)
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	unlicensed := testhelper.Setup(t, testhelper.WithoutLicense()).SiteActor(t, fixtures.OwnerJohnEmail)
	err = setRetention(t, unlicensed, fixtures.AcmePublicID, siteapi.NewNilRetentionDays(30))
	testhelper.SiteRefusal(t, err, http.StatusPaymentRequired)
	_, err = retentionOf(t, unlicensed, fixtures.AcmePublicID)
	testhelper.SiteRefusal(t, err, http.StatusPaymentRequired)
}
