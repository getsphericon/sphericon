package site_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func eraseByEmail(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, email string) error {
	t.Helper()
	return c.SiteContactsErase(context.Background(), siteapi.SiteContactsEraseParams{WorkspaceId: workspaceID, Email: email})
}

// Erase by address removes the Contact; a padded, mixed-case address finds it; a second
// lookup shows only the opt-outs Erasure keeps.
func TestSiteContactsEraseByEmailErasesTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	err := eraseByEmail(t, c, fixtures.AcmePublicID, " Export.Subject@Example.com ")
	require.NoError(t, err)

	still, err := env.DB.Contact.Query().Where(contact.Email(fixtures.ContactExportSubjectEmail)).Exist(context.Background())
	require.NoError(t, err)
	assert.False(t, still)

	got := lookup(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	assert.Equal(t, siteapi.SiteContactHoldings{Unsubscribes: 1, Suppressions: 1}, *got)
}

// An address with delivery records and no Contact is anonymized all the same.
func TestSiteContactsEraseByEmailAnonymizesDeliveryRecordsWithoutContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)

	err := eraseByEmail(t, c, fixtures.InitechPublicID, fixtures.OutboundMessageStrangerDestination)
	require.NoError(t, err)

	msg, err := env.DB.OutboundMessage.Get(context.Background(), fixtures.OutboundMessageStrangerID)
	require.NoError(t, err)
	assert.Empty(t, msg.Destination)
}

// An address nothing holds, and one of another Workspace, answer not-found.
func TestSiteContactsEraseByEmailNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	nobody := eraseByEmail(t, c, fixtures.AcmePublicID, "nobody@nowhere.test")
	foreign := eraseByEmail(t, c, fixtures.AcmePublicID, "pat@globex.test")

	testhelper.SiteRefusal(t, nobody, http.StatusNotFound)
	testhelper.SiteRefusal(t, foreign, http.StatusNotFound)
}

// An invalid address is rejected before anything is erased.
func TestSiteContactsEraseByEmailRejectsAnInvalidAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	err := eraseByEmail(t, c, fixtures.AcmePublicID, "not an address")
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// A plain member is refused and nothing is touched.
func TestSiteContactsEraseByEmailIsRefusedForMembers(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	refused := eraseByEmail(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	testhelper.SiteRefusal(t, refused, http.StatusForbidden)

	still, err := env.DB.Contact.Query().Where(contact.Email(fixtures.ContactExportSubjectEmail)).Exist(context.Background())
	require.NoError(t, err)
	assert.True(t, still)
}

// The contact.erased signal names the User who erased.
func TestSiteContactsEraseByEmailRecordsTheUserRequester(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	err := eraseByEmail(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	require.NoError(t, err)

	msgs := env.QueuedMessages(t, events.NameContactErased)
	require.Len(t, msgs, 1)
	assert.Equal(t, "user", msgs[0].Data["requesterKind"])
	assert.NotZero(t, msgs[0].Data["requesterId"])
}

// Erase by address leaves an Audit entry for the removed Contact that carries no address
// and no name (ADR 0021: the erasure must not re-store the PII); it keeps the actor.
func TestSiteContactsEraseByEmailAuditEntryHoldsNoAddress(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	err := eraseByEmail(t, c, fixtures.AcmePublicID, fixtures.ContactExportSubjectEmail)
	require.NoError(t, err)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, "contact.delete")

	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.NotContains(t, fmt.Sprintf("%+v", got[0]), fixtures.ContactExportSubjectEmail)
	assert.Empty(t, got[0].Target.Name.Value, "no name of the erased Contact")
}
