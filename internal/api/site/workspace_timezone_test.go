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

func setTimezone(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, name string) siteapi.SiteWorkspacesSetTimezoneRes {
	t.Helper()
	res, err := c.SiteWorkspacesSetTimezone(context.Background(),
		&siteapi.SiteWorkspaceTimezoneInput{Timezone: siteapi.TimeZoneName(name)},
		siteapi.SiteWorkspacesSetTimezoneParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func TestAWorkspaceReportsInUTCUntilAnOwnerSetsAZone(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list, err := owner.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, siteapi.TimeZoneName("UTC"), list[0].Timezone)
}

func TestAnOwnerSetsTheTimezoneAndItIsRecordedInTheAuditLog(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := setTimezone(t, owner, fixtures.AcmePublicID, "Europe/Berlin")
	updated, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.TimeZoneName("Europe/Berlin"), updated.Timezone)

	list, err := owner.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, siteapi.TimeZoneName("Europe/Berlin"), list[0].Timezone)

	published := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, published, 1)
	entry := published[0].(*events.AuditEntry)
	diff, err := json.Marshal(entry.Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"timezone":{"from":"UTC","to":"Europe/Berlin"}}`, string(diff))

	recorded := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceUpdate)
	require.Len(t, recorded, 1)
	assert.Equal(t, fixtures.OwnerJohnName, recorded[0].Actor.Name.Value)
}

func TestSettingTheSameTimezoneAgainRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	setTimezone(t, owner, fixtures.AcmePublicID, "Asia/Tokyo")
	setTimezone(t, owner, fixtures.AcmePublicID, "Asia/Tokyo")

	assert.Len(t, env.OutboxEvents(t, events.NameAuditEntry), 1)
}

func TestAMemberCannotSetTheTimezone(t *testing.T) {
	env := testhelper.Setup(t)

	member := setTimezone(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID, "Asia/Tokyo")
	outsider := setTimezone(t, env.SiteActor(t, fixtures.OutsiderOscarEmail), fixtures.AcmePublicID, "Asia/Tokyo")

	assert.IsType(t, &siteapi.SiteWorkspacesSetTimezoneForbidden{}, member)
	assert.IsType(t, &siteapi.SiteWorkspacesSetTimezoneNotFound{}, outsider)
	assert.Empty(t, env.OutboxEvents(t, events.NameAuditEntry))
}

func TestAnAdminSetsTheTimezone(t *testing.T) {
	env := testhelper.Setup(t)

	res := setTimezone(t, env.SiteActor(t, fixtures.AdminAliceEmail), fixtures.AcmePublicID, "Asia/Tokyo")

	updated, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.TimeZoneName("Asia/Tokyo"), updated.Timezone)
}

func TestAnUnknownTimezoneIsRefusedAndTheZoneStays(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := setTimezone(t, owner, fixtures.AcmePublicID, "Mars/Olympus")
	refused, ok := res.(*siteapi.SiteWorkspacesSetTimezoneUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Errors.Value["timezone"][0], "unknown time zone")

	list, err := owner.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, siteapi.TimeZoneName("UTC"), list[0].Timezone)
	assert.Empty(t, env.OutboxEvents(t, events.NameAuditEntry))
}
