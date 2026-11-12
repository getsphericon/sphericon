package server_test

import (
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A domain error of the Broadcasts and Templates modules reads the same on the site and on the
// external API: both render the module's own message, and none shows a package prefix. The
// refusal is the surface's problem, whose detail problemDetail reads.

var badDoc = map[string]jx.Raw{"type": jx.Raw(`"nope"`)}

func TestBroadcastRefusalsReadTheSameOnBothSurfaces(t *testing.T) {
	const missing = 99999
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	site := func(id int64) siteapi.EntityId { return siteapi.EntityId(id) }
	ext := func(id int64) externalapi.EntityId { return externalapi.EntityId(id) }
	at := time.Now().Add(time.Hour)

	cases := []struct {
		name string
		want string
		site func(c *siteapi.Client) error
		ext  func(c *externalapi.Client) error
	}{
		{
			name: "not found", want: "broadcast not found",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: ws, ID: site(missing)})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsGet(t.Context(), externalapi.BroadcastsGetParams{ID: ext(missing)})
				return err
			},
		},
		{
			name: "not a draft", want: "only draft broadcasts can be changed",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsUpdate(t.Context(), &siteapi.UpdateBroadcastInput{Revision: 1, Name: siteapi.NewOptName("x")},
					siteapi.SiteBroadcastsUpdateParams{WorkspaceId: ws, ID: site(fixtures.BroadcastSentID)})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsUpdate(t.Context(), &externalapi.UpdateBroadcastInput{Revision: 1, Name: externalapi.NewOptName("x")},
					externalapi.BroadcastsUpdateParams{ID: ext(fixtures.BroadcastSentID)})
				return err
			},
		},
		{
			name: "not sendable", want: "only draft or scheduled broadcasts can be sent or scheduled",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsSchedule(t.Context(), &siteapi.ScheduleBroadcastInput{Revision: 1, ScheduledAt: siteapi.Timestamp(at)},
					siteapi.SiteBroadcastsScheduleParams{WorkspaceId: ws, ID: site(fixtures.BroadcastSentID)})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsSchedule(t.Context(), &externalapi.ScheduleBroadcastInput{Revision: 1, ScheduledAt: externalapi.Timestamp(at)},
					externalapi.BroadcastsScheduleParams{ID: ext(fixtures.BroadcastSentID)})
				return err
			},
		},
		{
			name: "not archivable", want: "a scheduled or sending broadcast cannot be archived",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsArchive(t.Context(), siteapi.SiteBroadcastsArchiveParams{WorkspaceId: ws, ID: site(fixtures.BroadcastScheduledID)})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsArchive(t.Context(), externalapi.BroadcastsArchiveParams{ID: ext(fixtures.BroadcastScheduledID)})
				return err
			},
		},
		{
			name: "blank name", want: "name must not be empty",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{Name: " ", Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{})},
					siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{Name: " ", Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{})})
				return err
			},
		},
		{
			name: "body and document together", want: "write a body or a doc, not both",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
					Name:    "x",
					Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString("<p>x</p>"), Doc: siteapi.NewOptEditorDocument(badDoc)}),
				}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{
					Name:    "x",
					Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptEditorDocument(badDoc)}),
				})
				return err
			},
		},
		{
			name: "unknown segment", want: "segment not found",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
					Name:      "x",
					Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
					SegmentId: siteapi.NewOptNilEntityId(missing),
				}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.BroadcastsSetSegment(t.Context(), &externalapi.SetBroadcastSegmentInput{Revision: 1, SegmentId: externalapi.NewNilEntityId(missing)},
					externalapi.BroadcastsSetSegmentParams{ID: ext(fixtures.BroadcastDraftID)})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			err := tc.site(env.SiteActor(t, fixtures.OwnerJohnEmail))
			assert.Equal(t, tc.want, problemDetail(t, err), "site")
			err = tc.ext(env.ExternalScoped(t, "broadcasts:read", "broadcasts:write", "broadcasts:send"))
			assert.Equal(t, tc.want, problemDetail(t, err), "external")
		})
	}
}

// The site names the Integration of a Broadcast; the external API has no field for it.
func TestBroadcastUnknownIntegrationReadsWithoutAPrefix(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
		Name:          "x",
		Content:       testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		IntegrationId: siteapi.NewOptNilEntityId(99999),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	assert.Equal(t, "integration not found", problemDetail(t, err))
}

// The site has no way to unschedule a Broadcast, so only the external API can refuse it.
func TestBroadcastNotScheduledReadsWithoutAPrefix(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.ExternalScoped(t, "broadcasts:send").BroadcastsUnschedule(t.Context(),
		externalapi.BroadcastsUnscheduleParams{ID: fixtures.BroadcastDraftID})
	assert.Equal(t, "only scheduled broadcasts can be unscheduled", problemDetail(t, err))
}

func TestTemplateRefusalsReadTheSameOnBothSurfaces(t *testing.T) {
	const missing = 99999
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	cases := []struct {
		name string
		want string
		site func(c *siteapi.Client) error
		ext  func(c *externalapi.Client) error
	}{
		{
			name: "not found", want: "template not found",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteTemplatesGet(t.Context(), siteapi.SiteTemplatesGetParams{WorkspaceId: ws, ID: missing})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.TemplatesGet(t.Context(), externalapi.TemplatesGetParams{ID: missing})
				return err
			},
		},
		{
			name: "blank name", want: "name must not be empty",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{Name: ""},
					siteapi.SiteTemplatesCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{Name: ""})
				return err
			},
		},
		{
			name: "body and document together", want: "write a body or a doc, not both",
			site: func(c *siteapi.Client) error {
				_, err := c.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{
					Name: "x", Body: siteapi.NewOptString("<p>x</p>"), Doc: siteapi.NewOptEditorDocument(badDoc),
				}, siteapi.SiteTemplatesCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(c *externalapi.Client) error {
				_, err := c.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{
					Name: "x", Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptEditorDocument(badDoc),
				})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			err := tc.site(env.SiteActor(t, fixtures.OwnerJohnEmail))
			assert.Equal(t, tc.want, problemDetail(t, err), "site")
			err = tc.ext(env.ExternalScoped(t, "templates:read", "templates:write"))
			assert.Equal(t, tc.want, problemDetail(t, err), "external")
		})
	}
}

// An invalid document is refused with the reason the renderer gives, which both surfaces show.
func TestInvalidContentReadsTheSameOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	site := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ext := env.ExternalScoped(t, "templates:write", "broadcasts:write")
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	_, err1 := site.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
		Name:    "x",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Doc: siteapi.NewOptEditorDocument(badDoc)}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
	_, err2 := ext.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{
		Name:    "x",
		Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Doc: externalapi.NewOptEditorDocument(badDoc)}),
	})
	_, err3 := site.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{
		Name: "x", Doc: siteapi.NewOptEditorDocument(badDoc),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: ws})
	_, err4 := ext.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{
		Name: "x", Doc: externalapi.NewOptEditorDocument(badDoc),
	})

	want := problemDetail(t, err1)
	assert.Contains(t, want, "cannot be rendered")
	assert.Equal(t, want, problemDetail(t, err2))
	assert.Equal(t, want, problemDetail(t, err3))
	assert.Equal(t, want, problemDetail(t, err4))
}
