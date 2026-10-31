package staff_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// The Sign-up items and their Legal documents (ADR 0062) are a core section of the console: every
// test here runs on an Instance without a License key.

// console is the core /staff client of the enrolled fixture Staff member, signed in.
func (h *harness) console(t *testing.T) *staffapi.Client {
	t.Helper()
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)
	return h.env.StaffWithToken(t, cookie.Value)
}

// markMailings brings the fixture item id into effect as the one that stands for consent to the
// operator's mailings. The mark moves the service feed of Users, so it is written through
// env.FeedDB, as the bus's transaction of the console writes it.
func (h *harness) markMailings(t *testing.T, id int64) {
	t.Helper()
	h.env.PutSignupItemsInEffect(t.Context(), id)
	require.NoError(t, h.env.FeedDB.SignupItem.UpdateOneID(id).SetMailingsConsent(true).Exec(t.Context()))
}

// addItem adds a Sign-up item through the console and returns it.
func addItem(t *testing.T, c *staffapi.Client, in staffapi.StaffSignupItemInput) *staffapi.StaffSignupItemResource {
	t.Helper()
	res, err := c.StaffSignupItemsCreate(t.Context(), &in)
	require.NoError(t, err)
	item, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the item is added, got %T", res)
	return item
}

// saveDraft writes the item's draft through the console.
func saveDraft(t *testing.T, c *staffapi.Client, id staffapi.EntityId, markdown string) *staffapi.StaffSignupItemResource {
	t.Helper()
	res, err := c.StaffSignupItemsSaveDraft(t.Context(), &staffapi.StaffLegalDocumentDraftInput{Markdown: staffapi.LegalDocumentMarkdown(markdown)}, staffapi.StaffSignupItemsSaveDraftParams{ID: id})
	require.NoError(t, err)
	item, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the draft is saved, got %T", res)
	return item
}

// publish publishes the item's draft under label and returns the answer.
func publish(t *testing.T, c *staffapi.Client, id staffapi.EntityId, label string) staffapi.StaffSignupItemsPublishRes {
	t.Helper()
	res, err := c.StaffSignupItemsPublish(t.Context(), &staffapi.StaffLegalDocumentPublishInput{Label: staffapi.LegalDocumentLabel(label)}, staffapi.StaffSignupItemsPublishParams{ID: id})
	require.NoError(t, err)
	return res
}

// published publishes the item's draft under label, which must succeed.
func published(t *testing.T, c *staffapi.Client, id staffapi.EntityId, label string) *staffapi.StaffSignupItemResource {
	t.Helper()
	res := publish(t, c, id, label)
	item, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the version is published, got %T", res)
	return item
}

// visibleItems is what a visitor's sign-up form reads.
func (h *harness) visibleItems(t *testing.T) []siteapi.SiteSignupItem {
	t.Helper()
	res, err := h.env.SiteAnonymous(t).SiteLegalDocumentsSignupItems(t.Context())
	require.NoError(t, err)
	return res.Items
}

// publicDocument is what a visitor reads on the public page of key.
func (h *harness) publicDocument(t *testing.T, key string) siteapi.SiteLegalDocumentsGetRes {
	t.Helper()
	res, err := h.env.SiteAnonymous(t).SiteLegalDocumentsGet(t.Context(), siteapi.SiteLegalDocumentsGetParams{Key: siteapi.SignupItemKey(key)})
	require.NoError(t, err)
	return res
}

// readDocument is the public page of key, which must be served.
func (h *harness) readDocument(t *testing.T, key string) *siteapi.SiteLegalDocument {
	t.Helper()
	res := h.publicDocument(t, key)
	doc, ok := res.(*siteapi.SiteLegalDocument)
	require.Truef(t, ok, "the document is served, got %T", res)
	return doc
}

// refusedFields is the fields a 422 of the console names.
func refusedFields(t *testing.T, res any) []string {
	t.Helper()
	p := unprocessable(t, res)
	return lo.Keys(p.Errors.Value)
}

// unprocessable is the problem of a 422 the console answered with; anything else fails the test.
func unprocessable(t *testing.T, res any) staffapi.ProblemDetails {
	t.Helper()
	switch p := res.(type) {
	case *staffapi.StaffSignupItemsCreateUnprocessableEntity:
		return staffapi.ProblemDetails(*p)
	case *staffapi.StaffSignupItemsUpdateUnprocessableEntity:
		return staffapi.ProblemDetails(*p)
	case *staffapi.StaffSignupItemsPublishUnprocessableEntity:
		return staffapi.ProblemDetails(*p)
	case *staffapi.StaffSignupItemsReorderUnprocessableEntity:
		return staffapi.ProblemDetails(*p)
	case *staffapi.StaffSignupItemsArchiveUnprocessableEntity:
		return staffapi.ProblemDetails(*p)
	}
	require.Failf(t, "not a 422", "got %T", res)
	return staffapi.ProblemDetails{}
}

// request sends a bodiless request with the given method to the server.
func request(t *testing.T, h http.Handler, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWithNoSignupItemsTheAnonymousItemsReadIsEmpty(t *testing.T) {
	h := newHarness(t)

	items := h.visibleItems(t)

	assert.Empty(t, items)
}

func TestAnAddedItemWithoutAPublishedVersionIsListedInTheConsoleButNotToVisitors(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	item := addItem(t, c, staffapi.StaffSignupItemInput{Key: "terms", Title: "Terms of service", Kind: staffapi.SignupItemKindRequiredAcceptance})
	saveDraft(t, c, item.ID, "# Terms")

	assert.EqualValues(t, "terms", item.Key)
	assert.False(t, item.CurrentVersion.Set, "nothing is published yet")
	list, err := c.StaffSignupItemsList(t.Context())
	require.NoError(t, err)
	listed, ok := list.(*staffapi.StaffSignupItemList)
	require.True(t, ok)
	added, ok := lo.Find(listed.Items, func(it staffapi.StaffSignupItemResource) bool { return it.Key == "terms" })
	require.True(t, ok, "the added item is listed")
	assert.EqualValues(t, "Terms of service", added.Title)
	visible := h.visibleItems(t)
	assert.Empty(t, visible, "an item without a published version is not asked")
	assert.IsType(t, &siteapi.SiteLegalDocumentsGetNotFound{}, h.publicDocument(t, "terms"), "a draft has no public page")
}

// The acceptance path of #736: a Staff member without a License key adds an item, writes a draft,
// publishes it, and the anonymous reads return it.
func TestAPublishedItemIsAskedAndItsDocumentIsReadAnonymously(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	notice := addItem(t, c, staffapi.StaffSignupItemInput{Key: "privacy-notice", Title: "Privacy notice", Kind: staffapi.SignupItemKindNotice})
	saveDraft(t, c, notice.ID, "# Privacy\n\nWe keep your address.")

	item := published(t, c, notice.ID, "2026-10")

	require.True(t, item.CurrentVersion.Set)
	assert.EqualValues(t, "2026-10", item.CurrentVersion.Value.Label)
	visible := h.visibleItems(t)
	assert.Equal(t, []siteapi.SiteSignupItem{{Key: "privacy-notice", Title: "Privacy notice", Kind: siteapi.SignupItemKindNotice, Version: "2026-10"}}, visible)
	doc := h.readDocument(t, "privacy-notice")
	assert.EqualValues(t, "2026-10", doc.Version)
	assert.Equal(t, "Privacy notice", doc.Title)
	assert.EqualValues(t, "# Privacy\n\nWe keep your address.", doc.Markdown)
}

func TestTheItemsAreAskedInTheOrderTheyWereAdded(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	for _, key := range []string{"terms", "privacy", "news"} {
		it := addItem(t, c, staffapi.StaffSignupItemInput{Key: staffapi.SignupItemKey(key), Title: staffapi.SignupItemTitle(key), Kind: staffapi.SignupItemKindOptionalConsent})
		saveDraft(t, c, it.ID, "text of "+key)
		published(t, c, it.ID, "v1")
	}

	visible := h.visibleItems(t)

	asked := lo.Map(visible, func(it siteapi.SiteSignupItem, _ int) siteapi.SignupItemKey { return it.Key })
	assert.Equal(t, []siteapi.SignupItemKey{"terms", "privacy", "news"}, asked)
}

func TestEditingTheDraftChangesNothingAVisitorSeesUntilItIsPublished(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	before := h.readDocument(t, fixtures.ServiceTermsKey)

	edited := saveDraft(t, c, fixtures.ServiceTermsID, "third text")

	assert.EqualValues(t, "third text", edited.Draft.Value)
	assert.EqualValues(t, fixtures.ServiceTermsCurrentLabel, edited.CurrentVersion.Value.Label)
	unchanged := h.readDocument(t, fixtures.ServiceTermsKey)
	assert.Equal(t, before, unchanged)
	assert.EqualValues(t, fixtures.ServiceTermsCurrentLabel, h.visibleItems(t)[0].Version)

	published(t, c, fixtures.ServiceTermsID, "2026-03")

	after := h.readDocument(t, fixtures.ServiceTermsKey)
	assert.EqualValues(t, "third text", after.Markdown)
	assert.EqualValues(t, "2026-03", after.Version)
	assert.EqualValues(t, "2026-03", h.visibleItems(t)[0].Version)
}

func TestAPublishedVersionCannotBeRepublishedUnderItsLabel(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	before := h.readDocument(t, fixtures.ServiceTermsKey)
	saveDraft(t, c, fixtures.ServiceTermsID, "a new text")

	res := publish(t, c, fixtures.ServiceTermsID, fixtures.ServiceTermsCurrentLabel)

	assert.Equal(t, []string{"label"}, refusedFields(t, res))
	after := h.readDocument(t, fixtures.ServiceTermsKey)
	assert.Equal(t, before, after, "the published version is unchanged")
}

func TestThereIsNoOperationThatChangesOrRemovesAPublishedVersion(t *testing.T) {
	h := newHarness(t)
	cookie := h.session(t, fixtures.StaffEnrolledEmail, fixtures.StaffEnrolledPassword, fixtures.StaffEnrolledTotpSecret)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	before := h.readDocument(t, fixtures.ServiceTermsKey)

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := request(t, h.env.Server, method, "/staff/signup-items/"+strconv.FormatInt(fixtures.ServiceTermsID, 10)+"/versions", cookie)
		assert.Contains(t, []int{http.StatusNotFound, http.StatusMethodNotAllowed}, rec.Code, method)
	}
	after := h.readDocument(t, fixtures.ServiceTermsKey)
	assert.Equal(t, before, after)
}

func TestPublishingAnItemWithoutADraftIsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	res := publish(t, c, fixtures.ServiceTermsID, "2026-03")

	unprocessable(t, res)
	assert.EqualValues(t, fixtures.ServiceTermsCurrentLabel, h.visibleItems(t)[0].Version, "no version is published")
}

func TestATakenKeyIsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	res, err := c.StaffSignupItemsCreate(t.Context(), &staffapi.StaffSignupItemInput{Key: fixtures.ServiceTermsKey, Title: "Other terms", Kind: staffapi.SignupItemKindNotice})
	require.NoError(t, err)

	assert.Equal(t, []string{"key"}, refusedFields(t, res))
}

func TestAMailingsConsentIsOnlyAnOptionalConsent(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.DataNoticeID)

	required, err := c.StaffSignupItemsCreate(t.Context(), &staffapi.StaffSignupItemInput{Key: "terms", Title: "Terms", Kind: staffapi.SignupItemKindRequiredAcceptance, MailingsConsent: staffapi.NewOptBool(true)})
	require.NoError(t, err)
	marked := update(t, c, fixtures.DataNoticeID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})

	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, required))
	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, marked))
}

func TestASecondMailingsConsentIsRefused(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.markMailings(t, fixtures.OperatorNewsID)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.PartnerOffersID)

	added, err := c.StaffSignupItemsCreate(t.Context(), &staffapi.StaffSignupItemInput{Key: "events", Title: "Events", Kind: staffapi.SignupItemKindOptionalConsent, MailingsConsent: staffapi.NewOptBool(true)})
	require.NoError(t, err)
	marked := update(t, c, fixtures.PartnerOffersID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})

	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, added))
	assert.Equal(t, []string{"mailingsConsent"}, refusedFields(t, marked))
	news := getItem(t, c, fixtures.OperatorNewsID)
	assert.True(t, news.MailingsConsent, "the first item keeps the mark")
}

func TestTheMailingsConsentMovesOnceItIsUnmarked(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)
	h.markMailings(t, fixtures.OperatorNewsID)
	h.env.PutSignupItemsInEffect(t.Context(), fixtures.PartnerOffersID)

	update(t, c, fixtures.OperatorNewsID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(false)})
	res := update(t, c, fixtures.PartnerOffersID, staffapi.StaffSignupItemUpdate{MailingsConsent: staffapi.NewOptBool(true)})

	moved, ok := res.(*staffapi.StaffSignupItemResource)
	require.Truef(t, ok, "the mark moves, got %T", res)
	assert.True(t, moved.MailingsConsent)
}

func TestAnUnknownItemIsNotFound(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	res, err := c.StaffSignupItemsGet(t.Context(), staffapi.StaffSignupItemsGetParams{ID: 999999})
	require.NoError(t, err)

	assert.IsType(t, &staffapi.StaffSignupItemsGetNotFound{}, res)
}

func TestTheSignupItemsNeedAStaffSession(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.StaffAnonymous(t).StaffSignupItemsList(t.Context())
	require.NoError(t, err)

	requireProblem(t, res, http.StatusUnauthorized)
}
