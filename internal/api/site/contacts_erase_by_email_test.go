package site_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func eraseByEmail(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, email string) siteapi.SiteContactsEraseRes {
	t.Helper()
	res, err := c.SiteContactsErase(context.Background(), siteapi.SiteContactsEraseParams{WorkspaceId: workspaceID, Email: email})
	require.NoError(t, err)
	return res
}

// Erase by address removes the Contact; a padded, mixed-case address finds it; a second
// lookup shows only the opt-outs Erasure keeps.
func TestSiteContactsEraseByEmailErasesTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := eraseByEmail(t, c, fixtures.AcmePublicID, " Export.Subject@Example.com ")
	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, res)

	still, err := env.DB.Contact.Query().Where(contact.Email(fixtures.ContactExportSubjectEmail)).Exist(context.Background())
	require.NoError(t, err)
	assert.False(t, still)

	got, ok := lookup(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail).(*siteapi.SiteContactHoldings)
	require.True(t, ok)
	assert.Equal(t, siteapi.SiteContactHoldings{Unsubscribes: 1, Suppressions: 1}, *got)
}

// An address with delivery records and no Contact is anonymized all the same.
func TestSiteContactsEraseByEmailAnonymizesDeliveryRecordsWithoutContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)

	res := eraseByEmail(t, c, fixtures.InitechPublicID, fixtures.OutboundMessageStrangerDestination)
	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, res)

	msg, err := env.DB.OutboundMessage.Get(context.Background(), fixtures.OutboundMessageStrangerID)
	require.NoError(t, err)
	assert.Empty(t, msg.Destination)
}

// An address nothing holds, and one of another Workspace, answer not-found.
func TestSiteContactsEraseByEmailNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	assert.IsType(t, &siteapi.SiteContactsEraseNotFound{}, eraseByEmail(t, c, fixtures.AcmePublicID, "nobody@nowhere.test"))
	assert.IsType(t, &siteapi.SiteContactsEraseNotFound{}, eraseByEmail(t, c, fixtures.AcmePublicID, "pat@globex.test"))
}

// An invalid address is rejected before anything is erased.
func TestSiteContactsEraseByEmailRejectsAnInvalidAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	assert.IsType(t, &siteapi.SiteContactsEraseBadRequest{}, eraseByEmail(t, c, fixtures.AcmePublicID, "not an address"))
}

// A plain member is refused and nothing is touched.
func TestSiteContactsEraseByEmailIsRefusedForMembers(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	res := eraseByEmail(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	assert.IsType(t, &siteapi.SiteContactsEraseForbidden{}, res)

	still, err := env.DB.Contact.Query().Where(contact.Email(fixtures.ContactExportSubjectEmail)).Exist(context.Background())
	require.NoError(t, err)
	assert.True(t, still)
}

// The contact.erased signal names the User who erased.
func TestSiteContactsEraseByEmailRecordsTheUserRequester(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := eraseByEmail(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, res)

	msgs := env.Outbox(t, events.NameContactErased)
	require.Len(t, msgs, 1)
	assert.Equal(t, "user", msgs[0].Data["requesterKind"])
	assert.NotZero(t, msgs[0].Data["requesterId"])
}

// Erase by address leaves an Audit entry for the removed Contact that carries no address
// and no name (ADR 0021: the erasure must not re-store the PII); it keeps the actor.
func TestSiteContactsEraseByEmailAuditEntryHoldsNoAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.IsType(t, &siteapi.SiteContactsEraseNoContent{}, eraseByEmail(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail))

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, "contact.delete")

	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.NotContains(t, fmt.Sprintf("%+v", got[0]), fixtures.ContactExportSubjectEmail)
	assert.Empty(t, got[0].Target.Name.Value, "no name of the erased Contact")
}
