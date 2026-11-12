package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// guardCase is one id-addressed operation; every guard refuses it with the same status: a
// missing scope with 403, a missing credential with 401 and a malformed id with 400.
type guardCase struct {
	name string
	call func(c *externalapi.Client, id externalapi.EntityId) error
}

func idGuardCases() []guardCase {
	ctx := context.Background()
	return []guardCase{
		{"AutomationsGet", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
			return err
		}},
		{"AutomationsUpdate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: 1}, externalapi.AutomationsUpdateParams{ID: id})
			return err
		}},
		{"AutomationsArchive", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: id})
			return err
		}},
		{"AutomationsPublish", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: 1}, externalapi.AutomationsPublishParams{ID: id})
			return err
		}},
		{"AutomationsDeactivate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
			return err
		}},
		{"BroadcastsGet", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: id})
			return err
		}},
		{"BroadcastsUpdate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: 1}, externalapi.BroadcastsUpdateParams{ID: id})
			return err
		}},
		{"BroadcastsSetSegment", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: 1, SegmentId: externalapi.NilEntityId{Null: true}}, externalapi.BroadcastsSetSegmentParams{ID: id})
			return err
		}},
		{"BroadcastsSchedule", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{Revision: 1, ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))}, externalapi.BroadcastsScheduleParams{ID: id})
			return err
		}},
		{"BroadcastsUnschedule", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.BroadcastsUnschedule(ctx, externalapi.BroadcastsUnscheduleParams{ID: id})
			return err
		}},
		{"BroadcastsTestSend", func(c *externalapi.Client, id externalapi.EntityId) error {
			return c.BroadcastsTestSend(ctx, &externalapi.TestSendBroadcastInput{Email: externalapi.NewOptEmailAddress("qa@example.com")}, externalapi.BroadcastsTestSendParams{ID: id})
		}},
		{"BroadcastsReport", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: id})
			return err
		}},
		{"ContactsGet", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.ContactsGet(ctx, externalapi.ContactsGetParams{ID: id})
			return err
		}},
		{"ContactsUpdate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.ContactsUpdate(ctx, &externalapi.UpdateContactInput{Revision: 1}, externalapi.ContactsUpdateParams{ID: id})
			return err
		}},
		{"ContactsDelete", func(c *externalapi.Client, id externalapi.EntityId) error {
			return c.ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: id})
		}},
		{"SegmentsGet", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: id})
			return err
		}},
		{"SegmentsUpdate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Revision: 1}, externalapi.SegmentsUpdateParams{ID: id})
			return err
		}},
		{"SegmentsArchive", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: id})
			return err
		}},
		{"SegmentsUnarchive", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.SegmentsUnarchive(ctx, externalapi.SegmentsUnarchiveParams{ID: id})
			return err
		}},
		{"TagsListForContact", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.TagsListForContact(ctx, externalapi.TagsListForContactParams{ContactId: id})
			return err
		}},
		{"TagsApply", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.TagsApply(ctx, &externalapi.ApplyTagInput{Name: "vip"}, externalapi.TagsApplyParams{ContactId: id})
			return err
		}},
		{"TagsRemove", func(c *externalapi.Client, id externalapi.EntityId) error {
			return c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: id, Name: "vip"})
		}},
		{"TemplatesGet", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: id})
			return err
		}},
		{"TemplatesUpdate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: 1}, externalapi.TemplatesUpdateParams{ID: id})
			return err
		}},
		{"TemplatesDelete", func(c *externalapi.Client, id externalapi.EntityId) error {
			return c.TemplatesDelete(ctx, externalapi.TemplatesDeleteParams{ID: id})
		}},
		{"WebhooksGet", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.WebhooksGet(ctx, externalapi.WebhooksGetParams{ID: id})
			return err
		}},
		{"WebhooksUpdate", func(c *externalapi.Client, id externalapi.EntityId) error {
			_, err := c.WebhooksUpdate(ctx, &externalapi.UpdateWebhookInput{Revision: 1}, externalapi.WebhooksUpdateParams{ID: id})
			return err
		}},
		{"WebhooksDelete", func(c *externalapi.Client, id externalapi.EntityId) error {
			return c.WebhooksDelete(ctx, externalapi.WebhooksDeleteParams{ID: id})
		}},
	}
}

// An id that is no public id of this Instance is refused (a 400 problem) before any handler runs,
// for every id-addressed operation: of the wrong form, or of the right form and not sealed under
// the Instance's id key. The typed client cannot spell one, so each call carries a sentinel id
// whose public form is swapped on the wire.
func TestExternalMalformedIDIsABadRequest(t *testing.T) {
	env := testhelper.Setup(t)
	bearer := env.ScopedBearer(t, apitokens.Scopes...)
	for _, bad := range []string{"abc", "99999999999999999999", "1.5", "-", "AAAAAAAAAAAAAAAAAAAAAA"} {
		c := env.ExternalVia(t, bearer, testhelper.RewriteURL(testhelper.ID(123456789), bad))
		for _, tc := range idGuardCases() {
			t.Run(tc.name+"/"+bad, func(t *testing.T) {
				err := tc.call(c, 123456789)
				testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			})
		}
	}
}

// A token holding only an unrelated scope is refused by every id-addressed
// operation before the id is even looked at.
func TestExternalIDOperationsRequireTheirScope(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTokensRead)
	for _, tc := range idGuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(c, 1)
			testhelper.ExternalRefusal(t, err, http.StatusForbidden)
		})
	}
}

// An anonymous caller never reaches a handler.
func TestExternalIDOperationsRequireACredential(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalAnonymous(t)
	for _, tc := range idGuardCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call(c, 1)
			testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)
		})
	}
}
