package external_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func smsConsentOf(contactID int64) externalapi.SmsConsentsGetParams {
	return externalapi.SmsConsentsGetParams{ContactId: externalapi.EntityId(contactID)}
}

// An API client (and so an agent over MCP) reads a Contact's SMS consent as the Contact card does:
// the Confirmation with its provenance and evidence, then the opt-out that ends it (ADR 0077).
// Krusty has a phone of each: no consent yet, a Confirmation, and an opt-out recorded by hand.
func TestAnAPIClientReadsAContactsSMSConfirmationAndOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.KrustyID, "contacts:read"))

	before, err := c.SmsConsentsGet(ctx, smsConsentOf(fixtures.ContactKrustyTexterID))
	require.NoError(t, err)
	confirmed, err := c.SmsConsentsGet(ctx, smsConsentOf(fixtures.ContactKrustyConfirmedID))
	require.NoError(t, err)
	stopped, err := c.SmsConsentsGet(ctx, smsConsentOf(fixtures.ContactKrustyOptedOutID))
	require.NoError(t, err)

	none, ok := before.(*externalapi.SmsConsent)
	require.Truef(t, ok, "got %T", before)
	assert.Equal(t, fixtures.ContactKrustyTexterPhone, none.Phone.Value)
	assert.False(t, none.Confirmation.Set, "no consent yet")
	assert.False(t, none.OptOut.Set, "no opt-out yet")

	consent, ok := confirmed.(*externalapi.SmsConsent)
	require.Truef(t, ok, "got %T", confirmed)
	require.True(t, consent.Confirmation.Set)
	evidence := consent.Confirmation.Value
	assert.Equal(t, externalapi.ConfirmationProvenanceImported, evidence.Provenance)
	assert.Equal(t, "Yes, text me offers from Krusty. Reply STOP to opt out.", evidence.Wording.Value)
	assert.Equal(t, "checkout form", evidence.Source.Value)
	assert.Equal(t, "Krusty", evidence.Brand.Value)
	confirmedAt := time.Time(evidence.ConfirmedAt)
	assert.False(t, confirmedAt.IsZero(), "when the number consented")

	optedOut, ok := stopped.(*externalapi.SmsConsent)
	require.Truef(t, ok, "got %T", stopped)
	require.True(t, optedOut.OptOut.Set, "opted out")
	assert.Equal(t, externalapi.OptOutViaManual, optedOut.OptOut.Value.Via)
	assert.False(t, optedOut.Confirmation.Set, "the opt-out ended the consent")
}

func TestReadingSMSConsentNeedsContactsReadAndStaysInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	writer, err := env.ExternalScoped(t, "contacts:write").SmsConsentsGet(ctx, smsConsentOf(fixtures.ContactTexterID))
	require.NoError(t, err)
	foreign, err := env.ExternalScoped(t, "contacts:read").SmsConsentsGet(ctx, smsConsentOf(fixtures.TyrellMatrixConfirmedID))
	require.NoError(t, err)

	assert.IsType(t, new(externalapi.SmsConsentsGetForbidden), writer)
	assert.IsType(t, new(externalapi.SmsConsentsGetNotFound), foreign)
}
