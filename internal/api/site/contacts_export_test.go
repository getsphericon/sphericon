package site_test

import (
	"context"
	"io"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// readSiteExport decodes the streamed body as the contract's ContactExportDocument,
// which fails on a missing member or a malformed value.
func readSiteExport(t *testing.T, res siteapi.SiteContactsExportRes) siteapi.ContactExportDocument {
	t.Helper()
	ok, isOK := res.(*siteapi.SiteContactsExportOKApplicationOctetStreamHeaders)
	require.Truef(t, isOK, "got %T", res)
	assert.Contains(t, ok.ContentDisposition, "attachment")
	raw, err := io.ReadAll(ok.Response)
	require.NoError(t, err)
	var doc siteapi.ContactExportDocument
	require.NoError(t, doc.Decode(jx.DecodeBytes(raw)), string(raw))
	require.NoError(t, doc.Validate())
	return doc
}

// Exporting Contacts is the Permission contacts:export (ADR 0061): an admin runs a subject-access
// request, a plain member is refused.
func TestSiteContactsExportIsForOwnersAndAdmins(t *testing.T) {
	env := testhelper.Setup(t)
	params := siteapi.SiteContactsExportParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          siteapi.NewOptEntityId(siteapi.EntityId(800)),
	}

	res, err := env.SiteActor(t, fixtures.AdminAliceEmail).SiteContactsExport(context.Background(), params)
	require.NoError(t, err)
	doc := readSiteExport(t, res)
	assert.Len(t, doc.Events, 3)
	assert.Len(t, doc.OutboundMessages, 3)
	assert.Len(t, doc.BroadcastRecipients, 1)

	refused, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteContactsExport(context.Background(), params)
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteContactsExportForbidden{}, refused)
	assert.Equal(t, siteapi.ProblemCodePermissionRequired, refused.(*siteapi.SiteContactsExportForbidden).Code.Value)
}

func TestSiteContactsExportByEmail(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := owner.SiteContactsExport(context.Background(), siteapi.SiteContactsExportParams{
		WorkspaceId: fixtures.AcmePublicID,
		Email:       siteapi.NewOptEmailAddress(fixtures.ContactExportSubjectEmail),
	})
	require.NoError(t, err)
	doc := readSiteExport(t, res)

	assert.Equal(t, siteapi.EntityId(800), doc.Contact.ID)
	assert.Equal(t, fixtures.ContactExportSubjectEmail, doc.Contact.Email.Value)
}

func TestSiteContactsExportStaysInsideTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	// A foreign Contact is not found through the caller's own Workspace.
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := owner.SiteContactsExport(ctx, siteapi.SiteContactsExportParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          siteapi.NewOptEntityId(siteapi.EntityId(900)),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsExportNotFound{}, res)

	// A non-member cannot address the Workspace at all.
	outsider := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	res, err = outsider.SiteContactsExport(ctx, siteapi.SiteContactsExportParams{
		WorkspaceId: fixtures.AcmePublicID,
		ID:          siteapi.NewOptEntityId(siteapi.EntityId(800)),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsExportNotFound{}, res)
}
