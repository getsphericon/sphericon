package signupitems_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func setup(t *testing.T) (*testhelper.TestEnv, *signupitems.Module) {
	t.Helper()
	env := testhelper.Setup(t)
	return env, signupitems.New(env.DB, env.Bus, time.Now)
}

// A key another item has, a deleted one's too, is refused by signup_items_key_key, and nothing is
// added.
func TestAddRefusesATakenKey(t *testing.T) {
	env, m := setup(t)
	before, err := env.DB.SignupItem.Query().Count(t.Context())
	require.NoError(t, err)

	_, err = m.Add(t.Context(), signupitems.Input{Key: fixtures.ServiceTermsKey, Title: "Other terms", Kind: signupitem.KindNotice})

	require.ErrorIs(t, err, signupitems.ErrKeyTaken)
	after, err := env.DB.SignupItem.Query().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// One item stands for consent to the operator's mailings: signupitem_mailings_consent refuses a
// second, added or marked, and the first keeps the mark.
func TestASecondMailingsConsentIsRefused(t *testing.T) {
	env, m := setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID, fixtures.PartnerOffersID)
	_, err := m.Update(t.Context(), fixtures.OperatorNewsID, signupitems.UpdateInput{MailingsConsent: new(true)})
	require.NoError(t, err)

	_, added := m.Add(t.Context(), signupitems.Input{Key: "events", Title: "Events", Kind: signupitem.KindOptionalConsent, MailingsConsent: true})
	_, marked := m.Update(t.Context(), fixtures.PartnerOffersID, signupitems.UpdateInput{MailingsConsent: new(true)})

	require.ErrorIs(t, added, signupitems.ErrMailingsTaken)
	require.ErrorIs(t, marked, signupitems.ErrMailingsTaken)
	news, err := m.Get(t.Context(), fixtures.OperatorNewsID)
	require.NoError(t, err)
	assert.True(t, news.MailingsConsent, "the first item keeps the mark")
}

// A label the item has already published under is refused by legaldocumentversion_item_id_label.
func TestPublishRefusesALabelTheItemHas(t *testing.T) {
	env, m := setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	_, err := m.SaveDraft(t.Context(), fixtures.ServiceTermsID, "a new text")
	require.NoError(t, err)

	_, err = m.Publish(t.Context(), fixtures.ServiceTermsID, fixtures.ServiceTermsCurrentLabel, fixtures.StaffEnrolledID)

	require.ErrorIs(t, err, signupitems.ErrLabelTaken)
	doc, err := m.Document(t.Context(), fixtures.ServiceTermsKey)
	require.NoError(t, err)
	assert.NotEqual(t, "a new text", doc.Markdown, "the published version is unchanged")
}
