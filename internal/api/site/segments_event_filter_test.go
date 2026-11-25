package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Event filter of an event test (ADR 0153): in Dunder, the Newsletter's /pricing link was
// clicked by DunderPricingClicker, its other link by DunderBlogClicker, the same link in the Promo
// by DunderPromoClicker and in an archived Broadcast by DunderArchivedClicker; three Orders were
// completed, paid, pending and with no status.

// clickedIn is an event test of a click of the Broadcast id whose link holds pricing, at least once.
func clickedIn(id int64, extra string) string {
	return `{"kind":"event","action":"email.clicked","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","entries":[` +
		`{"kind":"sentBy","sender":"broadcast","id":"` + publicid.Encode(id) + `"}` + extra + `]}}`
}

const pricingLink = `,{"kind":"property","key":"url","test":{"kind":"text","operator":"contains","value":"/PRICING"}}`

// createDunderSegment saves, in Dunder, a Segment of one leaf.
func createDunderSegment(t *testing.T, c *siteapi.Client, name, leaf string) (*siteapi.SiteSegmentResource, error) {
	t.Helper()
	def := `{"kind":"group","combinator":"and","entries":[` + leaf + `]}`
	return c.SiteSegmentsCreate(context.Background(), &siteapi.CreateSegmentInput{
		Name:      siteapi.Name(name),
		Condition: testhelper.ConditionInput[siteapi.SegmentConditionInput](t, def),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.DunderPublicID})
}

// dunderMembers is the emails of the Contacts a stored Dunder Segment matches.
func dunderMembers(t *testing.T, c *siteapi.Client, seg *siteapi.SiteSegmentResource) []string {
	t.Helper()
	page, err := c.SiteSegmentContactsList(context.Background(), siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.DunderPublicID, SegmentId: seg.ID,
	})
	require.NoError(t, err)
	return lo.Map(page.Items, func(ct siteapi.SiteContactResource, _ int) string { return string(ct.Email.Or("")) })
}

func TestSiteSegmentCountsTheClicksOfOneLinkOfOneBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)

	seg, err := createDunderSegment(t, c, "Pricing in the newsletter", clickedIn(fixtures.BroadcastDunderNewsletterID, pricingLink))
	require.NoError(t, err)
	members := dunderMembers(t, c, seg)
	anyLink := previewOne(t, c, clickedIn(fixtures.BroadcastDunderNewsletterID, ""))

	assert.Equal(t, []string{fixtures.DunderPricingClickerEmail}, members, "not another link of it, nor the link in the Promo")
	assert.Equal(t, int32(2), anyLink.Count, "every link of the newsletter")
}

func TestSiteSegmentCountsOnlyTheEventsWhosePropertiesPass(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	const paid = `{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}}`

	paidOnly, err := createDunderSegment(t, c, "Paid", `{"kind":"event","action":"order.completed","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","entries":[`+paid+`]}}`)
	require.NoError(t, err)
	notPaid, err := createDunderSegment(t, c, "Not paid", `{"kind":"event","action":"order.completed","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","not":true,"entries":[`+paid+`]}}`)
	require.NoError(t, err)
	paidMembers, notPaidMembers := dunderMembers(t, c, paidOnly), dunderMembers(t, c, notPaid)

	assert.Equal(t, []string{fixtures.DunderPaidBuyerEmail}, paidMembers)
	assert.ElementsMatch(t, []string{fixtures.DunderPendingBuyerEmail, fixtures.DunderStatuslessBuyerEmail}, notPaidMembers,
		"an Order without a status fails the test, so the negation holds for it")
}

// An archived Broadcast keeps its Events: a filter naming it still matches who clicked in it, and a
// new Segment may name it. Archiving one a Segment names is not refused.
func TestSiteSegmentNamesAnArchivedBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	ctx := context.Background()

	named, err := createDunderSegment(t, c, "Clicked the promo", clickedIn(fixtures.BroadcastDunderPromoID, pricingLink))
	require.NoError(t, err)
	_, archiveErr := c.SiteBroadcastsArchive(ctx, siteapi.SiteBroadcastsArchiveParams{WorkspaceId: fixtures.DunderPublicID, ID: idStr(fixtures.BroadcastDunderPromoID)})
	stillMatched := dunderMembers(t, c, named)
	archived, err := createDunderSegment(t, c, "Clicked last month", clickedIn(fixtures.BroadcastDunderArchivedID, pricingLink))
	require.NoError(t, err)
	archivedMembers := dunderMembers(t, c, archived)

	require.NoError(t, archiveErr)
	assert.Equal(t, []string{fixtures.DunderPromoClickerEmail}, stillMatched)
	assert.Equal(t, []string{fixtures.DunderArchivedClickerEmail}, archivedMembers)
}

// A Broadcast or Automation the Workspace does not have is refused when the Segment is saved, and so
// is a sent-by test of an action that no message sends.
func TestSiteSegmentRefusesASenderItCannotName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)

	_, otherWorkspaces := createDunderSegment(t, c, "Acme's", clickedIn(fixtures.BroadcastArchivedID, ""))
	_, noAutomation := createDunderSegment(t, c, "No such automation",
		`{"kind":"event","action":"email.sent","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","entries":[{"kind":"sentBy","sender":"automation","id":"`+publicid.Encode(999999)+`"}]}}`)
	_, notAMessage := createDunderSegment(t, c, "Ordered from the newsletter",
		`{"kind":"event","action":"order.completed","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","entries":[{"kind":"sentBy","sender":"broadcast","id":"`+publicid.Encode(fixtures.BroadcastDunderNewsletterID)+`"}]}}`)

	refused := testhelper.SiteRefusal(t, otherWorkspaces, http.StatusBadRequest)
	testhelper.SiteRefusal(t, noAutomation, http.StatusBadRequest)
	testhelper.SiteRefusal(t, notAMessage, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "entry 0: filter: entry 0:", "the refusal names the path of the test")
}

// A Tag named in an event test's filter is in Definition use: deleting it is refused and names the
// Segment.
func TestSiteTagDeleteRefusedWhileAnEventFilterNamesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	_, err := createDunderSegment(t, c, "Became a trial",
		`{"kind":"group","combinator":"or","entries":[{"kind":"event","action":"contact.tagged","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","entries":[{"kind":"property","key":"tag","test":{"kind":"text","operator":"eq","value":"trial"}}]}}]}`)
	require.NoError(t, err)

	deleteErr := c.SiteTagsDelete(context.Background(), siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.DunderPublicID, ID: idStr(fixtures.TagDunderTrialID)})

	refused := testhelper.SiteRefusal(t, deleteErr, http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindSegment: {"Became a trial"}}, usedBy(refused))
}

// A Custom field named in an event test's filter is in Definition use too: any retype is refused,
// since it is the change that removes the field the filter names, and names the Segment.
func TestSiteCustomFieldRetypeRefusedWhileAnEventFilterNamesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	_, err := createDunderSegment(t, c, "Changed plan",
		`{"kind":"event","action":"contact.field_changed","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"and","entries":[{"kind":"property","key":"key","test":{"kind":"text","operator":"eq","value":"plan"}}]}}`)
	require.NoError(t, err)
	plan := fieldByKey(t, c, fixtures.DunderPublicID, "plan")

	_, retypeErr := updateField(c, fixtures.DunderPublicID, plan.ID, &siteapi.UpdateCustomFieldInput{Revision: plan.Revision, Type: siteapi.NewOptCustomFieldType(siteapi.CustomFieldTypeNumber)})

	refused := testhelper.SiteRefusal(t, retypeErr, http.StatusConflict)
	assert.Equal(t, map[siteapi.UsedByKind][]string{siteapi.UsedByKindSegment: {"Changed plan"}}, usedBy(refused))
}

// An event test with no window, or one longer than the Events kept, counts only the Events kept,
// and the preview says how far back that is; a window inside it says nothing.
func TestSiteSegmentPreviewReportsTheRetentionHorizon(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)

	ever := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":1}`)
	longer := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":1,"withinDays":1000}`)
	inside := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":1,"withinDays":30}`)

	assert.Equal(t, siteapi.NewOptInt32(400), ever.EventsKeptDays, "the default SPHERICON_EVENTS_RETENTION, in days")
	assert.Equal(t, siteapi.NewOptInt32(400), longer.EventsKeptDays)
	assert.False(t, inside.EventsKeptDays.IsSet())
}

// A Rule's Condition and an Automation's Event filter take a sent-by test as a Segment's does: one
// naming a Broadcast of the Workspace is saved, one naming a Broadcast it does not have is refused
// (400), and so is one in the filter of an action no message sends (422).
func TestSiteRulesAndAutomationsNameWhatSentAMessage(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	ctx := context.Background()
	rule := func(leaf string) error {
		_, err := c.SiteRulesCreate(ctx, &siteapi.CreateRuleInput{
			Name: "Clickers", TriggerEvents: []string{"signed_up"},
			Condition: siteapi.NewOptRuleConditionInput(testhelper.ConditionInput[siteapi.RuleConditionInput](t, `{"kind":"group","combinator":"and","entries":[`+leaf+`]}`)),
			Actions:   []siteapi.RuleActionInput{tagAction(siteapi.RuleActionTypeApplyTag, "clicker")},
		}, siteapi.SiteRulesCreateParams{WorkspaceId: fixtures.DunderPublicID})
		return err
	}
	automation := func(action string, id int64) error {
		filter := testhelper.ConditionInput[siteapi.EventFilterInput](t,
			`{"kind":"group","combinator":"and","entries":[{"kind":"sentBy","sender":"broadcast","id":"`+publicid.Encode(id)+`"}]}`)
		_, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Clicked the newsletter", Trigger: testhelper.SiteTrigger(action, filter)},
			siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.DunderPublicID})
		return err
	}

	knownRule := rule(clickedIn(fixtures.BroadcastDunderNewsletterID, pricingLink))
	unknownRule := rule(clickedIn(fixtures.BroadcastArchivedID, ""))
	knownTrigger := automation("email.clicked", fixtures.BroadcastDunderNewsletterID)
	unknownTrigger := automation("email.clicked", fixtures.BroadcastArchivedID)
	orderTrigger := automation("order.completed", fixtures.BroadcastDunderNewsletterID)

	require.NoError(t, knownRule)
	testhelper.SiteRefusal(t, unknownRule, http.StatusBadRequest)
	require.NoError(t, knownTrigger)
	testhelper.SiteRefusal(t, unknownTrigger, http.StatusBadRequest)
	testhelper.SiteRefusal(t, orderTrigger, http.StatusUnprocessableEntity)
}
