package external_test

import (
	"context"
	"testing"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// guardCase is one id-addressed operation with the response each guard must yield.
type guardCase struct {
	name string
	call func(c *externalapi.Client, id externalapi.EntityId) (any, error)
	// forbidden, unauthorized and badID are the typed responses for a missing
	// scope (403), a missing credential (401) and a malformed id.
	forbidden, unauthorized, badID any
}

func idGuardCases() []guardCase {
	ctx := context.Background()
	return []guardCase{
		{"AutomationsGet", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
		}, &externalapi.AutomationsGetForbidden{}, &externalapi.AutomationsGetUnauthorized{}, &externalapi.AutomationsGetBadRequest{}},
		{"AutomationsUpdate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{}, externalapi.AutomationsUpdateParams{ID: id})
		}, &externalapi.AutomationsUpdateForbidden{}, &externalapi.AutomationsUpdateUnauthorized{}, &externalapi.AutomationsUpdateBadRequest{}},
		{"AutomationsDelete", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.AutomationsDelete(ctx, externalapi.AutomationsDeleteParams{ID: id})
		}, &externalapi.AutomationsDeleteForbidden{}, &externalapi.AutomationsDeleteUnauthorized{}, &externalapi.AutomationsDeleteBadRequest{}},
		{"AutomationsActivate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.AutomationsActivate(ctx, externalapi.AutomationsActivateParams{ID: id})
		}, &externalapi.AutomationsActivateForbidden{}, &externalapi.AutomationsActivateUnauthorized{}, &externalapi.AutomationsActivateBadRequest{}},
		{"AutomationsDeactivate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
		}, &externalapi.AutomationsDeactivateForbidden{}, &externalapi.AutomationsDeactivateUnauthorized{}, &externalapi.AutomationsDeactivateBadRequest{}},
		{"BroadcastsGet", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: id})
		}, &externalapi.BroadcastsGetForbidden{}, &externalapi.BroadcastsGetUnauthorized{}, &externalapi.BroadcastsGetBadRequest{}},
		{"BroadcastsUpdate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{}, externalapi.BroadcastsUpdateParams{ID: id})
		}, &externalapi.BroadcastsUpdateForbidden{}, &externalapi.BroadcastsUpdateUnauthorized{}, &externalapi.BroadcastsUpdateBadRequest{}},
		{"BroadcastsDelete", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsDelete(ctx, externalapi.BroadcastsDeleteParams{ID: id})
		}, &externalapi.BroadcastsDeleteForbidden{}, &externalapi.BroadcastsDeleteUnauthorized{}, &externalapi.BroadcastsDeleteBadRequest{}},
		{"BroadcastsSetSegment", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NilEntityId{Null: true}}, externalapi.BroadcastsSetSegmentParams{ID: id})
		}, &externalapi.BroadcastsSetSegmentForbidden{}, &externalapi.BroadcastsSetSegmentUnauthorized{}, &externalapi.BroadcastsSetSegmentBadRequest{}},
		{"BroadcastsSchedule", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))}, externalapi.BroadcastsScheduleParams{ID: id})
		}, &externalapi.BroadcastsScheduleForbidden{}, &externalapi.BroadcastsScheduleUnauthorized{}, &externalapi.BroadcastsScheduleBadRequest{}},
		{"BroadcastsUnschedule", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: id})
		}, &externalapi.BroadcastsUnscheduleForbidden{}, &externalapi.BroadcastsUnscheduleUnauthorized{}, &externalapi.BroadcastsUnscheduleBadRequest{}},
		{"BroadcastsTestSend", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: "qa@example.com"}, externalapi.BroadcastsTestSendParams{ID: id})
		}, &externalapi.BroadcastsTestSendForbidden{}, &externalapi.BroadcastsTestSendUnauthorized{}, &externalapi.BroadcastsTestSendBadRequest{}},
		{"BroadcastsReport", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: id})
		}, &externalapi.BroadcastsReportForbidden{}, &externalapi.BroadcastsReportUnauthorized{}, &externalapi.BroadcastsReportBadRequest{}},
		{"ContactsGet", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: id})
		}, &externalapi.ContactsGetForbidden{}, &externalapi.ContactsGetUnauthorized{}, &externalapi.ContactsGetBadRequest{}},
		{"ContactsUpdate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{}, externalapi.ContactsUpdateParams{ID: id})
		}, &externalapi.ContactsUpdateForbidden{}, &externalapi.ContactsUpdateUnauthorized{}, &externalapi.ContactsUpdateBadRequest{}},
		{"ContactsDelete", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: id})
		}, &externalapi.ContactsDeleteForbidden{}, &externalapi.ContactsDeleteUnauthorized{}, &externalapi.ContactsDeleteBadRequest{}},
		{"SegmentsGet", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: id})
		}, &externalapi.SegmentsGetForbidden{}, &externalapi.SegmentsGetUnauthorized{}, &externalapi.SegmentsGetBadRequest{}},
		{"SegmentsUpdate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{}, externalapi.SegmentsUpdateParams{ID: id})
		}, &externalapi.SegmentsUpdateForbidden{}, &externalapi.SegmentsUpdateUnauthorized{}, &externalapi.SegmentsUpdateBadRequest{}},
		{"SegmentsArchive", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: id})
		}, &externalapi.SegmentsArchiveForbidden{}, &externalapi.SegmentsArchiveUnauthorized{}, &externalapi.SegmentsArchiveBadRequest{}},
		{"SegmentsUnarchive", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.SegmentsUnarchive(ctx, externalapi.SegmentsUnarchiveParams{ID: id})
		}, &externalapi.SegmentsUnarchiveForbidden{}, &externalapi.SegmentsUnarchiveUnauthorized{}, &externalapi.SegmentsUnarchiveBadRequest{}},
		{"TagsListForContact", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: id})
		}, &externalapi.TagsListForContactForbidden{}, &externalapi.TagsListForContactUnauthorized{}, &externalapi.TagsListForContactBadRequest{}},
		{"TagsApply", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: id})
		}, &externalapi.TagsApplyForbidden{}, &externalapi.TagsApplyUnauthorized{}, &externalapi.TagsApplyBadRequest{}},
		{"TagsRemove", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: id, Name: "vip"})
		}, &externalapi.TagsRemoveForbidden{}, &externalapi.TagsRemoveUnauthorized{}, &externalapi.TagsRemoveBadRequest{}},
		{"TemplatesGet", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: id})
		}, &externalapi.TemplatesGetForbidden{}, &externalapi.TemplatesGetUnauthorized{}, &externalapi.TemplatesGetBadRequest{}},
		{"TemplatesUpdate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{}, externalapi.TemplatesUpdateParams{ID: id})
		}, &externalapi.TemplatesUpdateForbidden{}, &externalapi.TemplatesUpdateUnauthorized{}, &externalapi.TemplatesUpdateBadRequest{}},
		{"TemplatesDelete", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.TemplatesDelete(ctx, externalapi.TemplatesDeleteParams{ID: id})
		}, &externalapi.TemplatesDeleteForbidden{}, &externalapi.TemplatesDeleteUnauthorized{}, &externalapi.TemplatesDeleteBadRequest{}},
		{"WebhooksGet", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.WebhooksGet(ctx, externalapi.WebhooksGetParams{ID: id})
		}, &externalapi.WebhooksGetForbidden{}, &externalapi.WebhooksGetUnauthorized{}, &externalapi.WebhooksGetBadRequest{}},
		{"WebhooksUpdate", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{}, externalapi.WebhooksUpdateParams{ID: id})
		}, &externalapi.WebhooksUpdateForbidden{}, &externalapi.WebhooksUpdateUnauthorized{}, &externalapi.WebhooksUpdateBadRequest{}},
		{"WebhooksDelete", func(c *externalapi.Client, id externalapi.EntityId) (any, error) {
			return c.WebhooksDelete(ctx, externalapi.WebhooksDeleteParams{ID: id})
		}, &externalapi.WebhooksDeleteForbidden{}, &externalapi.WebhooksDeleteUnauthorized{}, &externalapi.WebhooksDeleteBadRequest{}},
	}
}

func allScopes() []string {
	return lo.Map(externalapi.ApiTokenScope("").AllValues(), func(s externalapi.ApiTokenScope, _ int) string { return string(s) })
}

// An id that is not an int64 is refused by the contract (a 400 problem) before any
// handler runs, for every id-addressed operation. The typed client cannot spell one, so
// each call carries a sentinel id that is swapped on the wire.
func TestExternalMalformedIDIsABadRequest(t *testing.T) {
	env := testhelper.Setup(t)
	bearer := env.ScopedBearer(t, allScopes()...)
	for _, bad := range []string{"abc", "99999999999999999999", "1.5", "-"} {
		c := env.ExternalVia(t, bearer, testhelper.RewriteURL("123456789", bad))
		for _, tc := range idGuardCases() {
			t.Run(tc.name+"/"+bad, func(t *testing.T) {
				res, err := tc.call(c, 123456789)
				require.NoError(t, err)
				assert.IsType(t, tc.badID, res)
			})
		}
	}
}

// A token holding only an unrelated scope is refused by every id-addressed
// operation before the id is even looked at.
func TestExternalIDOperationsRequireTheirScope(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "tokens:read")
	for _, tc := range idGuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.call(c, 1)
			require.NoError(t, err)
			assert.IsType(t, tc.forbidden, res)
		})
	}
}

// An anonymous caller never reaches a handler.
func TestExternalIDOperationsRequireACredential(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalAnonymous(t)
	for _, tc := range idGuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.call(c, 1)
			require.NoError(t, err)
			assert.IsType(t, tc.unauthorized, res)
		})
	}
}
