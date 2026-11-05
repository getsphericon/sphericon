package staff_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/legaldocumentversion"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Staff member keeps the Sign-up items in step with the market (#737): reorders, retitles and
// deletes them, and reads every published version of an item. The items are the fixture ones,
// brought into use where a test needs them.

// reorder sends the new order of the items and returns the answer.
func reorder(t *testing.T, c *staffapi.Client, ids ...staffapi.EntityId) (*staffapi.StaffSignupItemList, error) {
	t.Helper()
	return c.StaffSignupItemsReorder(t.Context(), &staffapi.StaffSignupItemOrder{Ids: ids})
}

// deleteItem deletes the item and returns the answer.
func deleteItem(t *testing.T, c *staffapi.Client, id staffapi.EntityId) error {
	t.Helper()
	return c.StaffSignupItemsDelete(t.Context(), staffapi.StaffSignupItemsDeleteParams{ID: id})
}

// deleted deletes the item, which must succeed.
func deleted(t *testing.T, c *staffapi.Client, id staffapi.EntityId) {
	t.Helper()
	require.NoError(t, deleteItem(t, c, id), "the item is deleted")
}

// listedKeys is the keys of the items the console lists, in their order.
func listedKeys(t *testing.T, c *staffapi.Client) []staffapi.SignupItemKey {
	t.Helper()
	list, err := c.StaffSignupItemsList(t.Context())
	require.NoError(t, err)
	return lo.Map(list.Items, func(it staffapi.StaffSignupItemResource, _ int) staffapi.SignupItemKey { return it.Key })
}

// tryUpdate changes the item and returns the answer.
func tryUpdate(t *testing.T, c *staffapi.Client, id staffapi.EntityId, u staffapi.StaffSignupItemUpdate) (*staffapi.StaffSignupItemResource, error) {
	t.Helper()
	return c.StaffSignupItemsUpdate(t.Context(), &u, staffapi.StaffSignupItemsUpdateParams{ID: id})
}

// update is tryUpdate for a change that is expected to be taken.
func update(t *testing.T, c *staffapi.Client, id staffapi.EntityId, u staffapi.StaffSignupItemUpdate) *staffapi.StaffSignupItemResource {
	t.Helper()
	item, err := tryUpdate(t, c, id, u)
	require.NoError(t, err)
	return item
}

// getItem is the item as the console shows it.
func getItem(t *testing.T, c *staffapi.Client, id staffapi.EntityId) *staffapi.StaffSignupItemResource {
	t.Helper()
	item, err := c.StaffSignupItemsGet(t.Context(), staffapi.StaffSignupItemsGetParams{ID: id})
	require.NoError(t, err)
	return item
}

// versions is the item's published versions as the console lists them.
func versions(t *testing.T, c *staffapi.Client, id staffapi.EntityId) []staffapi.StaffLegalDocumentVersion {
	t.Helper()
	list, err := c.StaffSignupItemsListVersions(t.Context(), staffapi.StaffSignupItemsListVersionsParams{ID: id})
	require.NoError(t, err)
	return list.Items
}

// labels is the labels of versions, in their order.
func labels(versions []staffapi.StaffLegalDocumentVersion) []staffapi.LegalDocumentLabel {
	return lo.Map(versions, func(v staffapi.StaffLegalDocumentVersion, _ int) staffapi.LegalDocumentLabel { return v.Label })
}

// askedKeys is the keys of the items a visitor's sign-up form asks, in its order.
func (h *harness) askedKeys(t *testing.T) []siteapi.SignupItemKey {
	t.Helper()
	visible := h.visibleItems(t)
	return lo.Map(visible, func(it siteapi.SiteSignupItem, _ int) siteapi.SignupItemKey { return it.Key })
}

// allInEffect brings every fixture item into use: the sign-up form asks ServiceTerms, OperatorNews
// and DataNotice, in that order, and PartnerOffers, which has no published version, is only listed.
func (h *harness) allInEffect(t *testing.T) {
	t.Helper()
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.OperatorNewsID, fixtures.DataNoticeID, fixtures.PartnerOffersID)
}

func TestReorderingChangesTheOrderTheFormAsks(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.allInEffect(t)

	list, err := reorder(t, c, fixtures.DataNoticeID, fixtures.OperatorNewsID, fixtures.ServiceTermsID, fixtures.PartnerOffersID)

	require.NoError(t, err)
	listed := lo.Map(list.Items, func(it staffapi.StaffSignupItemResource, _ int) staffapi.SignupItemKey { return it.Key })
	assert.Equal(t, []staffapi.SignupItemKey{fixtures.DataNoticeKey, fixtures.OperatorNewsKey, fixtures.ServiceTermsKey, fixtures.PartnerOffersKey}, listed)
	asked := h.askedKeys(t)
	assert.Equal(t, []siteapi.SignupItemKey{fixtures.DataNoticeKey, fixtures.OperatorNewsKey, fixtures.ServiceTermsKey}, asked)
}

func TestADeletedItemIsNotPartOfTheOrder(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	// DataNotice and PartnerOffers stay deleted, as the fixtures leave them.
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.OperatorNewsID)

	_, withDeleted := reorder(t, c, fixtures.OperatorNewsID, fixtures.DataNoticeID, fixtures.ServiceTermsID)
	list, err := reorder(t, c, fixtures.OperatorNewsID, fixtures.ServiceTermsID)

	assert.Equal(t, []string{"ids"}, refusedFields(t, withDeleted), "a deleted item is not in the order")
	require.NoError(t, err)
	listed := lo.Map(list.Items, func(it staffapi.StaffSignupItemResource, _ int) staffapi.SignupItemKey { return it.Key })
	assert.Equal(t, []staffapi.SignupItemKey{fixtures.OperatorNewsKey, fixtures.ServiceTermsKey}, listed)
	asked := h.askedKeys(t)
	assert.Equal(t, []siteapi.SignupItemKey{fixtures.OperatorNewsKey, fixtures.ServiceTermsKey}, asked)
}

func TestAnOrderThatIsNotEveryItemOnceIsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.allInEffect(t)
	every := []staffapi.EntityId{fixtures.ServiceTermsID, fixtures.OperatorNewsID, fixtures.DataNoticeID, fixtures.PartnerOffersID}
	orders := map[string][]staffapi.EntityId{
		"an item missing": every[1:],
		"an item twice":   append([]staffapi.EntityId{fixtures.OperatorNewsID}, every...),
		"an unknown item": append(every[1:], 999999),
		"no item":         {},
	}

	for name, ids := range orders {
		t.Run(name, func(t *testing.T) {
			_, err := reorder(t, c, ids...)

			assert.Equal(t, []string{"ids"}, refusedFields(t, err))
			asked := h.askedKeys(t)
			assert.Equal(t, []siteapi.SignupItemKey{fixtures.ServiceTermsKey, fixtures.OperatorNewsKey, fixtures.DataNoticeKey}, asked, "the order is unchanged")
		})
	}
}

func TestRetitlingChangesTheTitleVisitorsSeeWithoutANewVersion(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	item := update(t, c, fixtures.ServiceTermsID, staffapi.StaffSignupItemUpdate{Title: staffapi.NewOptSignupItemTitle("Terms of use")})

	assert.EqualValues(t, "Terms of use", item.Title)
	visible := h.visibleItems(t)
	assert.Equal(t, []siteapi.SiteSignupItem{{Key: fixtures.ServiceTermsKey, Title: "Terms of use", Kind: siteapi.SignupItemKindRequiredAcceptance, Version: fixtures.ServiceTermsCurrentLabel}}, visible)
	doc := h.readDocument(t, fixtures.ServiceTermsKey)
	assert.Equal(t, "Terms of use", doc.Title)
	listed := versions(t, c, fixtures.ServiceTermsID)
	assert.Equal(t, []staffapi.LegalDocumentLabel{fixtures.ServiceTermsCurrentLabel, fixtures.ServiceTermsStaleLabel}, labels(listed), "a new title is not a new version")
}

func TestABlankTitleIsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	before := h.visibleItems(t)

	_, err := tryUpdate(t, c, fixtures.ServiceTermsID, staffapi.StaffSignupItemUpdate{Title: staffapi.NewOptSignupItemTitle("   ")})

	assert.Equal(t, []string{"title"}, refusedFields(t, err))
	after := h.visibleItems(t)
	assert.Equal(t, before, after, "the title is unchanged")
}

func TestADeletedItemLeavesTheFormItsPageAndTheListButKeepsItsVersionsAndAnswers(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.DataNoticeID)

	deleted(t, c, fixtures.ServiceTermsID)

	asked := h.askedKeys(t)
	assert.Equal(t, []siteapi.SignupItemKey{fixtures.DataNoticeKey}, asked)
	testhelper.SiteRefusal(t, errOf(h.publicDocument(t, fixtures.ServiceTermsKey)), http.StatusNotFound, "a deleted item has no public page")
	assert.Equal(t, []staffapi.SignupItemKey{fixtures.DataNoticeKey}, listedKeys(t, c), "the console no longer lists it")
	item := getItem(t, c, fixtures.ServiceTermsID)
	assert.True(t, item.DiscardedAt.Set, "it is still read by id, deleted")
	listed := versions(t, c, fixtures.ServiceTermsID)
	assert.Equal(t, []staffapi.LegalDocumentLabel{fixtures.ServiceTermsCurrentLabel, fixtures.ServiceTermsStaleLabel}, labels(listed))
	answers, err := h.env.DB.ConsentRecord.Query().
		Where(consentrecord.HasVersionWith(legaldocumentversion.ItemID(fixtures.ServiceTermsID))).
		IDs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int64{fixtures.MaryTermsAcceptedID}, answers, "the Users' answers to it stay, so consent stays provable")
}

func TestDeletingADeletedItemKeepsTheMomentItWasDeleted(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	before := getItem(t, c, fixtures.DataNoticeID)
	require.True(t, before.DiscardedAt.Set, "the fixture item is deleted")

	deleted(t, c, fixtures.DataNoticeID)

	after := getItem(t, c, fixtures.DataNoticeID)
	assert.Equal(t, before.DiscardedAt, after.DiscardedAt)
}

func TestTheMailingsConsentIsNotDeletedWhileItIsMarked(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.markMailings(t, fixtures.OperatorNewsID)

	refused := deleteItem(t, c, fixtures.OperatorNewsID)
	kept := getItem(t, c, fixtures.OperatorNewsID)
	update(t, c, fixtures.OperatorNewsID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(false)})
	deleted(t, c, fixtures.OperatorNewsID)

	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, refused))
	assert.False(t, kept.DiscardedAt.Set, "the marked item is kept")
	unmarked := getItem(t, c, fixtures.OperatorNewsID)
	assert.True(t, unmarked.DiscardedAt.Set, "once unmarked it is deleted")
}

func TestADeletedItemCannotBeChanged(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	// ServiceTerms stays deleted, as the fixtures leave it.

	_, retitled := tryUpdate(t, c, fixtures.ServiceTermsID, staffapi.StaffSignupItemUpdate{Title: staffapi.NewOptSignupItemTitle("Terms of use")})
	_, marked := tryUpdate(t, c, fixtures.PartnerOffersID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})
	_, drafted := c.StaffSignupItemsSaveDraft(t.Context(), &staffapi.StaffLegalDocumentDraftInput{Markdown: "# Terms"}, staffapi.StaffSignupItemsSaveDraftParams{ID: fixtures.ServiceTermsID})
	_, published := c.StaffSignupItemsPublish(t.Context(), &staffapi.StaffLegalDocumentPublishInput{Label: "2026-11"}, staffapi.StaffSignupItemsPublishParams{ID: fixtures.ServiceTermsID})

	testhelper.StaffRefusal(t, retitled, http.StatusNotFound)
	testhelper.StaffRefusal(t, marked, http.StatusNotFound)
	testhelper.StaffRefusal(t, drafted, http.StatusNotFound)
	testhelper.StaffRefusal(t, published, http.StatusNotFound)
	item := getItem(t, c, fixtures.ServiceTermsID)
	assert.EqualValues(t, "Terms of service", item.Title, "the title is unchanged")
	listed := versions(t, c, fixtures.ServiceTermsID)
	assert.Equal(t, []staffapi.LegalDocumentLabel{fixtures.ServiceTermsCurrentLabel, fixtures.ServiceTermsStaleLabel}, labels(listed), "no version is published")
}

func TestTheVersionsAreListedLatestFirstWithWhoPublishedThem(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	listed := versions(t, c, fixtures.ServiceTermsID)

	assert.Equal(t, []staffapi.LegalDocumentLabel{fixtures.ServiceTermsCurrentLabel, fixtures.ServiceTermsStaleLabel}, labels(listed))
	publishers := lo.Map(listed, func(v staffapi.StaffLegalDocumentVersion, _ int) staffapi.EmailAddress { return v.PublishedBy.Email })
	assert.Equal(t, []staffapi.EmailAddress{fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledEmail}, publishers)
	item := getItem(t, c, fixtures.ServiceTermsID)
	assert.Equal(t, item.CurrentVersion.Value, listed[0], "the latest is the current version")
}

func TestTheVersionsOfAnUnknownItemAreNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	_, err := c.StaffSignupItemsListVersions(t.Context(), staffapi.StaffSignupItemsListVersionsParams{ID: 999999})

	testhelper.StaffRefusal(t, err, http.StatusNotFound)
}

func TestDeletingAnUnknownItemIsNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	err := deleteItem(t, c, 999999)

	testhelper.StaffRefusal(t, err, http.StatusNotFound)
}
