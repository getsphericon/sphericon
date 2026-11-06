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

func setTimezone(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, name string) *siteapi.SiteWorkspaceResource {
	t.Helper()
	res, err := trySetTimezone(t, c, workspaceID, name)
	require.NoError(t, err)
	return res
}

// trySetTimezone is setTimezone for a test that expects the refusal.
func trySetTimezone(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, name string) (*siteapi.SiteWorkspaceResource, error) {
	t.Helper()
	return c.SiteWorkspacesSetTimezone(context.Background(),
		&siteapi.SiteWorkspaceTimezoneInput{Timezone: siteapi.TimeZoneName(name)},
		siteapi.SiteWorkspacesSetTimezoneParams{WorkspaceId: workspaceID})
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

	updated := setTimezone(t, owner, fixtures.AcmePublicID, "Europe/Berlin")
	assert.Equal(t, siteapi.TimeZoneName("Europe/Berlin"), updated.Timezone)

	list, err := owner.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, siteapi.TimeZoneName("Europe/Berlin"), list[0].Timezone)

	published := env.QueuedEvents(t, events.NameAuditEntry)
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

	assert.Len(t, env.QueuedEvents(t, events.NameAuditEntry), 1)
}

func TestAMemberCannotSetTheTimezone(t *testing.T) {
	env := testhelper.Setup(t)

	_, member := trySetTimezone(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID, "Asia/Tokyo")
	_, outsider := trySetTimezone(t, env.SiteActor(t, fixtures.OutsiderOscarEmail), fixtures.AcmePublicID, "Asia/Tokyo")

	testhelper.SiteRefusal(t, member, http.StatusForbidden)
	testhelper.SiteRefusal(t, outsider, http.StatusNotFound)
	assert.Empty(t, env.QueuedEvents(t, events.NameAuditEntry))
}

func TestAnAdminSetsTheTimezone(t *testing.T) {
	env := testhelper.Setup(t)

	updated := setTimezone(t, env.SiteActor(t, fixtures.AdminAliceEmail), fixtures.AcmePublicID, "Asia/Tokyo")

	assert.Equal(t, siteapi.TimeZoneName("Asia/Tokyo"), updated.Timezone)
}

func TestAnUnknownTimezoneIsRefusedAndTheZoneStays(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := trySetTimezone(t, owner, fixtures.AcmePublicID, "Mars/Olympus")
	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Errors.Value["timezone"][0], "unknown time zone")

	list, err := owner.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Equal(t, siteapi.TimeZoneName("UTC"), list[0].Timezone)
	assert.Empty(t, env.QueuedEvents(t, events.NameAuditEntry))
}
