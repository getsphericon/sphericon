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
// archives them, and reads every published version of an item. The items are the fixture ones,
// brought into effect where a test needs them asked.

// reorder sends the new order of the items and returns the answer.
func reorder(t *testing.T, c *staffapi.Client, ids ...staffapi.EntityId) staffapi.StaffSignupItemsReorderRes {
	t.Helper()
	res, err := c.StaffSignupItemsReorder(t.Context(), &staffapi.StaffSignupItemOrder{Ids: ids})
	require.NoError(t, err)
	return res
}

// archive archives the item and returns the answer.
func archive(t *testing.T, c *staffapi.Client, id staffapi.EntityId) staffapi.StaffSignupItemsArchiveRes {
	t.Helper()
	res, err := c.StaffSignupItemsArchive(t.Context(), staffapi.StaffSignupItemsArchiveParams{ID: id})
	require.NoError(t, err)
	return res
}

// archived archives the item, which must succeed.
func archived(t *testing.T, c *staffapi.Client, id staffapi.EntityId) *staffapi.StaffSignupItemResource {
	t.Helper()
	res := archive(t, c, id)
	item, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the item is archived, got %T", res)
	return item
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

// allInEffect brings the fixture items that have a published version into effect: the sign-up form
// asks ServiceTerms, OperatorNews and DataNotice, in that order.
func (h *harness) allInEffect(t *testing.T) {
	t.Helper()
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.OperatorNewsID, fixtures.DataNoticeID)
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

func TestAnArchivedItemKeepsItsPlaceInTheOrder(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	// DataNotice stays archived, as the fixtures leave it.
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.OperatorNewsID)

	res := reorder(t, c, fixtures.OperatorNewsID, fixtures.DataNoticeID, fixtures.ServiceTermsID, fixtures.PartnerOffersID)

	list, ok := res.(*staffapi.StaffSignupItemList)
	require.Truef(t, ok, "the items are reordered, got %T", res)
	listed := lo.Map(list.Items, func(it staffapi.StaffSignupItemResource, _ int) staffapi.SignupItemKey { return it.Key })
	assert.Equal(t, []staffapi.SignupItemKey{fixtures.OperatorNewsKey, fixtures.DataNoticeKey, fixtures.ServiceTermsKey, fixtures.PartnerOffersKey}, listed)
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

func TestAnArchivedItemLeavesTheFormAndItsPageButKeepsItsVersions(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID, fixtures.DataNoticeID)

	item := archived(t, c, fixtures.ServiceTermsID)

	assert.True(t, item.ArchivedAt.Set, "the console shows the item archived")
	asked := h.askedKeys(t)
	assert.Equal(t, []siteapi.SignupItemKey{fixtures.DataNoticeKey}, asked)
	assert.IsType(t, &siteapi.SiteLegalDocumentsGetNotFound{}, h.publicDocument(t, fixtures.ServiceTermsKey), "an archived item has no public page")
	listed := versions(t, c, fixtures.ServiceTermsID)
	assert.Equal(t, []staffapi.LegalDocumentLabel{fixtures.ServiceTermsCurrentLabel, fixtures.ServiceTermsStaleLabel}, labels(listed))
}

func TestArchivingAnArchivedItemKeepsTheMomentItWasArchived(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	before := getItem(t, c, fixtures.DataNoticeID)
	require.True(t, before.ArchivedAt.Set, "the fixture item is archived")

	again := archived(t, c, fixtures.DataNoticeID)

	assert.Equal(t, before.ArchivedAt, again.ArchivedAt)
}

func TestTheMailingsConsentIsNotArchivedWhileItIsMarked(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.markMailings(t, fixtures.OperatorNewsID)

	refused := archive(t, c, fixtures.OperatorNewsID)
	update(t, c, fixtures.OperatorNewsID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(false)})
	unmarked := archived(t, c, fixtures.OperatorNewsID)

	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, refused))
	assert.True(t, unmarked.ArchivedAt.Set, "once unmarked it is archived")
}

func TestAnArchivedItemCannotBecomeTheMailingsConsent(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	// PartnerOffers stays archived, as the fixtures leave it.
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.OperatorNewsID)

	marked := update(t, c, fixtures.PartnerOffersID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})
	other := update(t, c, fixtures.OperatorNewsID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})

	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, marked))
	moved, ok := other.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "an item in use takes the mark, got %T", other)
	assert.True(t, moved.MailingsConsent)
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

func TestArchivingAnUnknownItemIsNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	res := archive(t, c, 999999)

	assert.IsType(t, &staffapi.StaffSignupItemsArchiveNotFound{}, res)
}
