package staff_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// A Staff member keeps the Sign-up items in step with the market (#737): reorders, retitles and
// deletes them, and reads every published version of an item. The items are the fixture ones,
// brought into use where a test needs them.

// reorder sends the new order of the items and returns the answer.
func reorder(t *testing.T, c *staffapi.Client, ids ...staffapi.EntityId) staffapi.StaffSignupItemsReorderRes {
	t.Helper()
	res, err := c.StaffSignupItemsReorder(t.Context(), &staffapi.StaffSignupItemOrder{Ids: ids})
	require.NoError(t, err)
	return res
}

// deleteItem deletes the item and returns the answer.
func deleteItem(t *testing.T, c *staffapi.Client, id staffapi.EntityId) staffapi.StaffSignupItemsDeleteRes {
	t.Helper()
	res, err := c.StaffSignupItemsDelete(t.Context(), staffapi.StaffSignupItemsDeleteParams{ID: id})
	require.NoError(t, err)
	return res
}

// deleted deletes the item, which must succeed.
func deleted(t *testing.T, c *staffapi.Client, id staffapi.EntityId) {
	t.Helper()
	res := deleteItem(t, c, id)
	require.IsTypef(t, &staffapi.StaffSignupItemsDeleteNoContent{}, res, "the item is deleted, got %T", res)
}

// listedKeys is the keys of the items the console lists, in their order.
func listedKeys(t *testing.T, c *staffapi.Client) []staffapi.SignupItemKey {
	t.Helper()
	res, err := c.StaffSignupItemsList(t.Context())
	require.NoError(t, err)
	list, ok := res.(*staffapi.StaffSignupItemList)
	require.Truef(t, ok, "the items are listed, got %T", res)
	return lo.Map(list.Items, func(it staffapi.StaffSignupItemResource, _ int) staffapi.SignupItemKey { return it.Key })
}

// update changes the item and returns the answer.
func update(t *testing.T, c *staffapi.Client, id staffapi.EntityId, u staffapi.StaffSignupItemUpdate) staffapi.StaffSignupItemsUpdateRes {
	t.Helper()
	res, err := c.StaffSignupItemsUpdate(t.Context(), &u, staffapi.StaffSignupItemsUpdateParams{ID: id})
	require.NoError(t, err)
	return res
}

// getItem is the item as the console shows it.
func getItem(t *testing.T, c *staffapi.Client, id staffapi.EntityId) *staffapi.StaffSignupItemResource {
	t.Helper()
	res, err := c.StaffSignupItemsGet(t.Context(), staffapi.StaffSignupItemsGetParams{ID: id})
	require.NoError(t, err)
	item, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the item is read, got %T", res)
	return item
}

// versions is the item's published versions as the console lists them.
func versions(t *testing.T, c *staffapi.Client, id staffapi.EntityId) []staffapi.StaffLegalDocumentVersion {
	t.Helper()
	res, err := c.StaffSignupItemsListVersions(t.Context(), staffapi.StaffSignupItemsListVersionsParams{ID: id})
	require.NoError(t, err)
	list, ok := res.(*staffapi.StaffLegalDocumentVersionList)
	require.Truef(t, ok, "the versions are listed, got %T", res)
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

	res := reorder(t, c, fixtures.DataNoticeID, fixtures.OperatorNewsID, fixtures.ServiceTermsID, fixtures.PartnerOffersID)

	list, ok := res.(*staffapi.StaffSignupItemList)
	require.Truef(t, ok, "the items are reordered, got %T", res)
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

	withDeleted := reorder(t, c, fixtures.OperatorNewsID, fixtures.DataNoticeID, fixtures.ServiceTermsID)
	res := reorder(t, c, fixtures.OperatorNewsID, fixtures.ServiceTermsID)

	assert.Equal(t, []string{"ids"}, refusedFields(t, withDeleted), "a deleted item is not in the order")
	list, ok := res.(*staffapi.StaffSignupItemList)
	require.Truef(t, ok, "the items are reordered, got %T", res)
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
			res := reorder(t, c, ids...)

			assert.Equal(t, []string{"ids"}, refusedFields(t, res))
			asked := h.askedKeys(t)
			assert.Equal(t, []siteapi.SignupItemKey{fixtures.ServiceTermsKey, fixtures.OperatorNewsKey, fixtures.DataNoticeKey}, asked, "the order is unchanged")
		})
	}
}

func TestRetitlingChangesTheTitleVisitorsSeeWithoutANewVersion(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	res := update(t, c, fixtures.ServiceTermsID, staffapi.StaffSignupItemUpdate{Title: staffapi.NewOptSignupItemTitle("Terms of use")})

	item, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the item is retitled, got %T", res)
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

	res := update(t, c, fixtures.ServiceTermsID, staffapi.StaffSignupItemUpdate{Title: staffapi.NewOptSignupItemTitle("   ")})

	assert.Equal(t, []string{"title"}, refusedFields(t, res))
	after := h.visibleItems(t)
	assert.Equal(t, before, after, "the title is unchanged")
}

func TestADeletedItemLeavesTheFormItsPageAndTheListButKeepsItsVersions(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.DataNoticeID)

	deleted(t, c, fixtures.ServiceTermsID)

	asked := h.askedKeys(t)
	assert.Equal(t, []siteapi.SignupItemKey{fixtures.DataNoticeKey}, asked)
	assert.IsType(t, &siteapi.SiteLegalDocumentsGetNotFound{}, h.publicDocument(t, fixtures.ServiceTermsKey), "a deleted item has no public page")
	assert.Equal(t, []staffapi.SignupItemKey{fixtures.DataNoticeKey}, listedKeys(t, c), "the console no longer lists it")
	item := getItem(t, c, fixtures.ServiceTermsID)
	assert.True(t, item.DiscardedAt.Set, "it is still read by id, deleted")
	listed := versions(t, c, fixtures.ServiceTermsID)
	assert.Equal(t, []staffapi.LegalDocumentLabel{fixtures.ServiceTermsCurrentLabel, fixtures.ServiceTermsStaleLabel}, labels(listed))
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

	retitled := update(t, c, fixtures.ServiceTermsID, staffapi.StaffSignupItemUpdate{Title: staffapi.NewOptSignupItemTitle("Terms of use")})
	marked := update(t, c, fixtures.PartnerOffersID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})
	drafted, err := c.StaffSignupItemsSaveDraft(t.Context(), &staffapi.StaffLegalDocumentDraftInput{Markdown: "# Terms"}, staffapi.StaffSignupItemsSaveDraftParams{ID: fixtures.ServiceTermsID})
	require.NoError(t, err)
	published, err := c.StaffSignupItemsPublish(t.Context(), &staffapi.StaffLegalDocumentPublishInput{Label: "2026-11"}, staffapi.StaffSignupItemsPublishParams{ID: fixtures.ServiceTermsID})
	require.NoError(t, err)

	assert.IsType(t, &staffapi.StaffSignupItemsUpdateNotFound{}, retitled)
	assert.IsType(t, &staffapi.StaffSignupItemsUpdateNotFound{}, marked)
	assert.IsType(t, &staffapi.StaffSignupItemsSaveDraftNotFound{}, drafted)
	assert.IsType(t, &staffapi.StaffSignupItemsPublishNotFound{}, published)
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

	res, err := c.StaffSignupItemsListVersions(t.Context(), staffapi.StaffSignupItemsListVersionsParams{ID: 999999})
	require.NoError(t, err)

	assert.IsType(t, &staffapi.StaffSignupItemsListVersionsNotFound{}, res)
}

func TestDeletingAnUnknownItemIsNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	res := deleteItem(t, c, 999999)

	assert.IsType(t, &staffapi.StaffSignupItemsDeleteNotFound{}, res)
}
