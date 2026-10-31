package server_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A domain error of the Broadcasts and Templates modules reads the same on the site and on the
// external API: both render the module's own message, and none shows a package prefix. The
// response types are per operation, so each case states how to read the detail of its response.

// detailOf is the problem detail of an ogen response, which every refusal carries.
func detailOf(t *testing.T, res any, err error) string {
	t.Helper()
	require.NoError(t, err)
	field := reflect.ValueOf(res).Elem().FieldByName("Detail")
	require.Truef(t, field.IsValid(), "%T carries no problem detail", res)
	detail, ok := reflect.TypeAssert[interface{ Get() (string, bool) }](field)
	require.Truef(t, ok, "%T detail is not optional", res)
	text, _ := detail.Get()
	return text
}

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
		site func(c *siteapi.Client) (any, error)
		ext  func(c *externalapi.Client) (any, error)
	}{
		{
			name: "not found", want: "broadcast not found",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: ws, ID: site(missing)})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsGet(t.Context(), externalapi.BroadcastsGetParams{ID: ext(missing)})
				return res, err
			},
		},
		{
			name: "not a draft", want: "only draft broadcasts can be changed",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsUpdate(t.Context(), &siteapi.UpdateBroadcastInput{Name: siteapi.NewOptString("x")},
					siteapi.SiteBroadcastsUpdateParams{WorkspaceId: ws, ID: site(fixtures.BroadcastSentID)})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsUpdate(t.Context(), &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString("x")},
					externalapi.BroadcastsUpdateParams{ID: ext(fixtures.BroadcastSentID)})
				return res, err
			},
		},
		{
			name: "not sendable", want: "only draft or scheduled broadcasts can be sent or scheduled",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsSchedule(t.Context(), &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(at)},
					siteapi.SiteBroadcastsScheduleParams{WorkspaceId: ws, ID: site(fixtures.BroadcastSentID)})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsSchedule(t.Context(), &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(at)},
					externalapi.BroadcastsScheduleParams{ID: ext(fixtures.BroadcastSentID)})
				return res, err
			},
		},
		{
			name: "not archivable", want: "only a sent or failed broadcast can be archived",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsArchive(t.Context(), siteapi.SiteBroadcastsArchiveParams{WorkspaceId: ws, ID: site(fixtures.BroadcastDraftID)})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsArchive(t.Context(), externalapi.BroadcastsArchiveParams{ID: ext(fixtures.BroadcastDraftID)})
				return res, err
			},
		},
		{
			name: "blank name", want: "name must not be empty",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{Name: " "},
					siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{Name: " "})
				return res, err
			},
		},
		{
			name: "body and document together", want: "write a body or a doc, not both",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
					Name: "x", Body: siteapi.NewOptString("<p>x</p>"), Doc: siteapi.NewOptCreateBroadcastInputDoc(badDoc),
				}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{
					Name: "x", Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptCreateBroadcastInputDoc(badDoc),
				})
				return res, err
			},
		},
		{
			name: "unknown segment", want: "segment not found",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
					Name: "x", SegmentId: siteapi.NewOptNilEntityId(missing),
				}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.BroadcastsSetSegment(t.Context(), &externalapi.SetBroadcastSegmentInput{SegmentId: externalapi.NewNilEntityId(missing)},
					externalapi.BroadcastsSetSegmentParams{ID: ext(fixtures.BroadcastDraftID)})
				return res, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			res, err := tc.site(env.SiteActor(t, fixtures.OwnerJohnEmail))
			assert.Equal(t, tc.want, detailOf(t, res, err), "site")
			res, err = tc.ext(env.ExternalScoped(t, "broadcasts:read", "broadcasts:write", "broadcasts:send"))
			assert.Equal(t, tc.want, detailOf(t, res, err), "external")
		})
	}
}

// The site names the Integration of a Broadcast; the external API has no field for it.
func TestBroadcastUnknownIntegrationReadsWithoutAPrefix(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
		Name: "x", IntegrationId: siteapi.NewOptNilEntityId(99999),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	assert.Equal(t, "integration not found", detailOf(t, res, err))
}

// The site has no way to unschedule a Broadcast, so only the external API can refuse it.
func TestBroadcastNotScheduledReadsWithoutAPrefix(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.ExternalScoped(t, "broadcasts:send").BroadcastsUnschedule(t.Context(),
		externalapi.BroadcastsUnscheduleParams{ID: fixtures.BroadcastDraftID})
	assert.Equal(t, "only scheduled broadcasts can be unscheduled", detailOf(t, res, err))
}

func TestTemplateRefusalsReadTheSameOnBothSurfaces(t *testing.T) {
	const missing = 99999
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	cases := []struct {
		name string
		want string
		site func(c *siteapi.Client) (any, error)
		ext  func(c *externalapi.Client) (any, error)
	}{
		{
			name: "not found", want: "template not found",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteTemplatesGet(t.Context(), siteapi.SiteTemplatesGetParams{WorkspaceId: ws, ID: missing})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.TemplatesGet(t.Context(), externalapi.TemplatesGetParams{ID: missing})
				return res, err
			},
		},
		{
			name: "blank name", want: "name must not be empty",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{Name: ""},
					siteapi.SiteTemplatesCreateParams{WorkspaceId: ws})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{Name: ""})
				return res, err
			},
		},
		{
			name: "body and document together", want: "write a body or a doc, not both",
			site: func(c *siteapi.Client) (any, error) {
				res, err := c.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{
					Name: "x", Body: siteapi.NewOptString("<p>x</p>"), Doc: siteapi.NewOptCreateTemplateInputDoc(badDoc),
				}, siteapi.SiteTemplatesCreateParams{WorkspaceId: ws})
				return res, err
			},
			ext: func(c *externalapi.Client) (any, error) {
				res, err := c.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{
					Name: "x", Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptCreateTemplateInputDoc(badDoc),
				})
				return res, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			res, err := tc.site(env.SiteActor(t, fixtures.OwnerJohnEmail))
			assert.Equal(t, tc.want, detailOf(t, res, err), "site")
			res, err = tc.ext(env.ExternalScoped(t, "templates:read", "templates:write"))
			assert.Equal(t, tc.want, detailOf(t, res, err), "external")
		})
	}
}

// An invalid document is refused with the reason the renderer gives, which both surfaces show.
func TestInvalidContentReadsTheSameOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	site := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ext := env.ExternalScoped(t, "templates:write", "broadcasts:write")
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	b1, err1 := site.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
		Name: "x", Doc: siteapi.NewOptCreateBroadcastInputDoc(badDoc),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: ws})
	b2, err2 := ext.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{
		Name: "x", Doc: externalapi.NewOptCreateBroadcastInputDoc(badDoc),
	})
	t1, err3 := site.SiteTemplatesCreate(t.Context(), &siteapi.CreateTemplateInput{
		Name: "x", Doc: siteapi.NewOptCreateTemplateInputDoc(badDoc),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: ws})
	t2, err4 := ext.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{
		Name: "x", Doc: externalapi.NewOptCreateTemplateInputDoc(badDoc),
	})

	want := detailOf(t, b1, err1)
	assert.Contains(t, want, "cannot be rendered")
	assert.Equal(t, want, detailOf(t, b2, err2))
	assert.Equal(t, want, detailOf(t, t1, err3))
	assert.Equal(t, want, detailOf(t, t2, err4))
}
