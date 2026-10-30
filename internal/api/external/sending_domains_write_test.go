package external_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func createDomain(t *testing.T, c *externalapi.Client, in externalapi.CreateSendingDomainInput) *externalapi.SendingDomainResource {
	t.Helper()
	res, err := c.SendingDomainsCreate(context.Background(), &in)
	require.NoError(t, err)
	created, ok := res.(*externalapi.SendingDomainResource)
	require.Truef(t, ok, "got %T", res)
	return created
}

func TestExternalSendingDomainsWritesNeedTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "sending_domains:read")
	id := entityIDString(fixtures.SendingDomainUnverifiedID)

	create, err := reader.SendingDomainsCreate(ctx, &externalapi.CreateSendingDomainInput{Domain: "x.example.com"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsCreateForbidden{}, create)

	upd, err := reader.SendingDomainsUpdate(ctx, &externalapi.UpdateSendingDomainInput{DkimSelector: externalapi.NewOptString("s2")}, externalapi.SendingDomainsUpdateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsUpdateForbidden{}, upd)

	ver, err := reader.SendingDomainsVerify(ctx, externalapi.SendingDomainsVerifyParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsVerifyForbidden{}, ver)

	archive, err := reader.SendingDomainsArchive(ctx, externalapi.SendingDomainsArchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsArchiveForbidden{}, archive)

	unarchive, err := reader.SendingDomainsUnarchive(ctx, externalapi.SendingDomainsUnarchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsUnarchiveForbidden{}, unarchive)

	writer := env.ExternalScoped(t, "sending_domains:write")
	get, err := writer.SendingDomainsGet(ctx, externalapi.SendingDomainsGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsGetForbidden{}, get, "write does not imply read")
}

func TestExternalSendingDomainShowsTheTrackingRecord(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")

	created := createDomain(t, c, externalapi.CreateSendingDomainInput{Domain: "marketing.example.com"})

	rec, ok := created.TrackingRecord.Get()
	require.True(t, ok, "the tracking record is listed")
	assert.Equal(t, externalapi.TrackingRecordTypeCNAME, rec.Type)
	assert.Equal(t, "track.marketing.example.com", rec.Host)
	assert.Equal(t, testhelper.TrackingCNAMETarget, rec.Target)
	assert.Equal(t, externalapi.TrackingStatePending, rec.State)
}

func TestExternalSendingDomainHasNoTrackingRecordWithoutATarget(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackingCNAMETarget = "" }))
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")

	created := createDomain(t, c, externalapi.CreateSendingDomainInput{Domain: "marketing.example.com"})

	assert.False(t, created.TrackingRecord.IsSet())
}

func TestExternalSendingDomainsCreateReturnsRecordsNeverTheKey(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")

	created := createDomain(t, c, externalapi.CreateSendingDomainInput{Domain: " Marketing.Example.COM. "})
	assert.Equal(t, "marketing.example.com", created.Domain)
	assert.Equal(t, "sphericon", created.DkimSelector)
	assert.False(t, created.Verified)
	assert.Equal(t, "sphericon._domainkey.marketing.example.com", created.DkimRecord.Host)
	assert.Contains(t, created.DkimRecord.Value, "v=DKIM1; k=rsa; p=")
	assert.Equal(t, "_dmarc.marketing.example.com", created.DmarcRecord.Host)

	row, err := env.DB.Scoped(fixtures.AcmeID).SendingDomain().Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.NotContains(t, row.DkimPrivateKeyEncrypted, "PRIVATE KEY", "the key is sealed at rest")

	get, err := c.SendingDomainsGet(ctx, externalapi.SendingDomainsGetParams{ID: created.ID})
	require.NoError(t, err)
	list, err := c.SendingDomainsList(ctx, externalapi.SendingDomainsListParams{})
	require.NoError(t, err)
	for name, v := range map[string]any{"create": created, "get": get, "list": list} {
		body, err := json.Marshal(v)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "PRIVATE KEY", name)
		assert.NotContains(t, string(body), row.DkimPrivateKeyEncrypted, name)
	}
}

func TestExternalSendingDomainsCreateValidatesAndRefusesDuplicates(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:write")

	for _, in := range []externalapi.CreateSendingDomainInput{
		{Domain: "not a domain"},
		{Domain: "localhost"},
		{Domain: "10.0.0.1"},
		{Domain: "ok.example.com", DkimSelector: externalapi.NewOptString("bad selector!")},
	} {
		res, err := c.SendingDomainsCreate(ctx, &in)
		require.NoError(t, err)
		assert.IsTypef(t, &externalapi.SendingDomainsCreateUnprocessableEntity{}, res, "%+v", in)
	}

	dup, err := c.SendingDomainsCreate(ctx, &externalapi.CreateSendingDomainInput{Domain: "MAIL.acme.com"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsCreateConflict{}, dup)
}

func TestExternalSendingDomainsUpdateMovesTheSelectorAndUnverifies(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")
	id := entityIDString(fixtures.SendingDomainVerifiedID)

	same, err := c.SendingDomainsUpdate(ctx, &externalapi.UpdateSendingDomainInput{}, externalapi.SendingDomainsUpdateParams{ID: id})
	require.NoError(t, err)
	assert.True(t, same.(*externalapi.SendingDomainResource).Verified, "an empty update changes nothing")

	res, err := c.SendingDomainsUpdate(ctx, &externalapi.UpdateSendingDomainInput{DkimSelector: externalapi.NewOptString("s2")}, externalapi.SendingDomainsUpdateParams{ID: id})
	require.NoError(t, err)
	updated, ok := res.(*externalapi.SendingDomainResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "s2", updated.DkimSelector)
	assert.Equal(t, "s2._domainkey.mail.acme.com", updated.DkimRecord.Host)
	assert.False(t, updated.Verified, "the new record is not published yet")
	assert.True(t, updated.VerificationLostAt.Set && !updated.VerificationLostAt.Null, "the loss is visible in the API")
	assert.True(t, updated.VerifiedAt.Set && !updated.VerifiedAt.Null, "when it was verified is kept")
	assert.Equal(t, fixtures.SendingDomainVerifiedDomain, updated.Domain, "the name is immutable")

	bad, err := c.SendingDomainsUpdate(ctx, &externalapi.UpdateSendingDomainInput{DkimSelector: externalapi.NewOptString("bad selector!")}, externalapi.SendingDomainsUpdateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsUpdateUnprocessableEntity{}, bad)
}

func TestExternalSendingDomainsVerifyRunsALiveCheckAndReportsIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")
	id := entityIDString(fixtures.SendingDomainUnverifiedID)

	res, err := c.SendingDomainsVerify(ctx, externalapi.SendingDomainsVerifyParams{ID: id})
	require.NoError(t, err)
	accepted, ok := res.(*externalapi.SendingDomainResource)
	require.Truef(t, ok, "got %T", res)
	assert.False(t, accepted.Verified, "the test DNS publishes nothing, and the caller cannot set verified")

	got, err := c.SendingDomainsGet(ctx, externalapi.SendingDomainsGetParams{ID: id})
	require.NoError(t, err)
	_, checked := got.(*externalapi.SendingDomainResource).LastCheckedAt.Get()
	assert.True(t, checked, "the live check ran and is reported")
}

func listedSendingDomainIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	res, err := c.SendingDomainsList(context.Background(), externalapi.SendingDomainsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*externalapi.SendingDomainsListOK)
	require.Truef(t, ok, "got %T", res)
	ids := make([]externalapi.EntityId, len(list.Items))
	for i := range list.Items {
		ids[i] = list.Items[i].ID
	}
	return ids
}

// /api archives, lists and restores a Sending domain; live work that sends from it refuses the
// archive with 409, and restoring one added again meanwhile is a 409.
func TestExternalSendingDomainsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")
	id := entityIDString(fixtures.SendingDomainUnverifiedID)

	out, err := c.SendingDomainsArchive(ctx, externalapi.SendingDomainsArchiveParams{ID: id})
	require.NoError(t, err)
	archived, ok := out.(*externalapi.SendingDomainResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.NotContains(t, listedSendingDomainIDs(t, c, false), id)
	assert.Contains(t, listedSendingDomainIDs(t, c, true), id)

	got, err := c.SendingDomainsGet(ctx, externalapi.SendingDomainsGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainResource{}, got, "an archived domain can still be opened")

	again, err := c.SendingDomainsCreate(ctx, &externalapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainUnverifiedDomain})
	require.NoError(t, err)
	created, ok := again.(*externalapi.SendingDomainResource)
	require.Truef(t, ok, "got %T", again)
	conflict, err := c.SendingDomainsUnarchive(ctx, externalapi.SendingDomainsUnarchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsUnarchiveConflict{}, conflict)

	_, err = c.SendingDomainsArchive(ctx, externalapi.SendingDomainsArchiveParams{ID: created.ID})
	require.NoError(t, err)
	back, err := c.SendingDomainsUnarchive(ctx, externalapi.SendingDomainsUnarchiveParams{ID: id})
	require.NoError(t, err)
	restored, ok := back.(*externalapi.SendingDomainResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedSendingDomainIDs(t, c, false), id)
}

func TestExternalSendingDomainsStayInTheirWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "sending_domains:read", "sending_domains:write")
	foreign := entityIDString(fixtures.SendingDomainGlobexID)

	get, err := c.SendingDomainsGet(ctx, externalapi.SendingDomainsGetParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsGetNotFound{}, get)

	upd, err := c.SendingDomainsUpdate(ctx, &externalapi.UpdateSendingDomainInput{DkimSelector: externalapi.NewOptString("s2")}, externalapi.SendingDomainsUpdateParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsUpdateNotFound{}, upd)

	ver, err := c.SendingDomainsVerify(ctx, externalapi.SendingDomainsVerifyParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsVerifyNotFound{}, ver)

	archive, err := c.SendingDomainsArchive(ctx, externalapi.SendingDomainsArchiveParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsArchiveNotFound{}, archive)

	unarchive, err := c.SendingDomainsUnarchive(ctx, externalapi.SendingDomainsUnarchiveParams{ID: foreign})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SendingDomainsUnarchiveNotFound{}, unarchive)

	n, err := env.DB.Scoped(fixtures.GlobexID).SendingDomain().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the foreign row survives")
}
