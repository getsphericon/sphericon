package site_test

import (
	"context"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func brandKitInput() *siteapi.SiteSaveBrandKitInput {
	return &siteapi.SiteSaveBrandKitInput{
		AccentColor:     "#FF5500",
		TextColor:       "#102030",
		BackgroundColor: "#f0f4f8",
		HeadingFont:     "Georgia, serif",
		BodyFont:        "Verdana, sans-serif",
		LogoUrl:         "https://cdn.example.com/logo.png",
		ButtonTextColor: "#fefefe",
		ButtonRadius:    12,
		FooterText:      "Vandelay Industries, 1 Main St",
	}
}

func saveBrandKit(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, in *siteapi.SiteSaveBrandKitInput) siteapi.SiteBrandKitSaveRes {
	t.Helper()
	res, err := c.SiteBrandKitSave(context.Background(), in, siteapi.SiteBrandKitSaveParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func getBrandKit(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) *siteapi.SiteBrandKitResource {
	t.Helper()
	res, err := c.SiteBrandKitGet(context.Background(), siteapi.SiteBrandKitGetParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	kit, ok := res.(*siteapi.SiteBrandKitResource)
	require.Truef(t, ok, "got %T", res)
	return kit
}

func TestSiteBrandKitIsTheDefaultsUntilItsFirstSave(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	kit := getBrandKit(t, c, fixtures.VandelayPublicID)
	assert.False(t, kit.Saved)
	assert.NotEmpty(t, kit.AccentColor)
	assert.NotEqual(t, "#cc0000", kit.AccentColor, "another Workspace's Brand kit is never shown")
	assert.False(t, kit.UpdatedAt.Set && !kit.UpdatedAt.Null)

	saved, ok := saveBrandKit(t, c, fixtures.VandelayPublicID, brandKitInput()).(*siteapi.SiteBrandKitResource)
	require.True(t, ok)
	assert.True(t, saved.Saved)
	assert.Equal(t, "#ff5500", saved.AccentColor, "colors are stored in lower case")
	assert.Equal(t, int32(12), saved.ButtonRadius)

	again := getBrandKit(t, c, fixtures.VandelayPublicID)
	assert.Equal(t, *saved, *again)
}

func TestSiteBrandKitFirstSaveMakesTheStarterTemplatesAndLaterSavesDoNot(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.VandelayID)
	before := s.EmailTemplate().Query().CountX(ctx)

	saveBrandKit(t, c, fixtures.VandelayPublicID, brandKitInput())

	all := s.EmailTemplate().Query().AllX(ctx)
	require.Len(t, all, before+4)
	starters := all[before:]
	var names []string
	for _, tpl := range starters {
		names = append(names, tpl.Name)
		assert.NotEmpty(t, tpl.Doc, "%s is written as a document", tpl.Name)
		assert.Contains(t, tpl.Body, "#f0f4f8", "%s carries the background", tpl.Name)
		assert.Contains(t, tpl.Body, "Verdana, sans-serif", "%s carries the body font", tpl.Name)
		assert.Contains(t, tpl.Body, "https://cdn.example.com/logo.png", "%s carries the logo", tpl.Name)
		assert.Contains(t, tpl.Body, "Vandelay Industries, 1 Main St", "%s carries the footer", tpl.Name)
	}
	assert.ElementsMatch(t, []string{"Welcome", "Receipt", "Digest", "One button"}, names)

	changed := brandKitInput()
	changed.AccentColor = "#00aa00"
	saveBrandKit(t, c, fixtures.VandelayPublicID, changed)

	assert.Equal(t, before+4, s.EmailTemplate().Query().CountX(ctx), "a later save makes no Template")
	assert.Equal(t, "#00aa00", getBrandKit(t, c, fixtures.VandelayPublicID).AccentColor)
	for _, tpl := range s.EmailTemplate().Query().AllX(ctx)[before:] {
		assert.NotContains(t, tpl.Body, "#00aa00", "the starters keep the brand they were made in")
	}
}

func TestSiteBrandKitSaveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	saveBrandKit(t, c, fixtures.VandelayPublicID, brandKitInput())
	changed := brandKitInput()
	changed.TextColor = "#000000"
	saveBrandKit(t, c, fixtures.VandelayPublicID, changed)

	entries := lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(e events.DomainEvent, _ int) *events.AuditEntry { return e.(*events.AuditEntry) })
	actions := lo.Map(entries, func(entry *events.AuditEntry, _ int) string { return entry.Action })
	updates := lo.Filter(entries, func(entry *events.AuditEntry, _ int) bool { return entry.Action == "brand_kit.update" })
	assert.Contains(t, actions, "brand_kit.create")
	require.NotEmpty(t, updates)
	update := updates[len(updates)-1]
	assert.Equal(t, map[string]any{"text_color": map[string]any{"from": "#102030", "to": "#000000"}}, update.Diff)
}

func TestSiteBrandKitRecreateMakesNewStartersInTheSavedBrandAndTouchesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	before := s.EmailTemplate().Query().AllX(ctx)

	res, err := c.SiteBrandKitRecreateStarterTemplates(ctx, siteapi.SiteBrandKitRecreateStarterTemplatesParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	made, ok := res.(*siteapi.SiteBrandKitRecreateStarterTemplatesCreatedApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, *made, 4)
	for _, tpl := range *made {
		assert.Contains(t, tpl.Body, "#fafafa", "made in Initech's brand")
		assert.Contains(t, tpl.Body, "Initech, 4120 Freidrich Lane")
	}
	assert.Len(t, s.EmailTemplate().Query().AllX(ctx), len(before)+4)

	// Recreating again adds another set and still rewrites nothing.
	_, err = c.SiteBrandKitRecreateStarterTemplates(ctx, siteapi.SiteBrandKitRecreateStarterTemplatesParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	assert.Len(t, s.EmailTemplate().Query().AllX(ctx), len(before)+8)
}

func TestSiteBrandKitRecreateNeedsASavedBrandKit(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	res, err := c.SiteBrandKitRecreateStarterTemplates(context.Background(), siteapi.SiteBrandKitRecreateStarterTemplatesParams{WorkspaceId: fixtures.VandelayPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBrandKitRecreateStarterTemplatesUnprocessableEntity{}, res)
}

// Editing the Brand kit never changes what is already written (ADR 0003, ADR 0040).
func TestSiteBrandKitEditChangesNoMessageAlreadyWritten(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.InitechID)
	_, err := c.SiteBrandKitRecreateStarterTemplates(ctx, siteapi.SiteBrandKitRecreateStarterTemplatesParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)

	type snapshot struct {
		templates  map[int64]string
		broadcasts map[int64]string
		automation map[int64]string
	}
	take := func() snapshot {
		snap := snapshot{templates: map[int64]string{}, broadcasts: map[int64]string{}, automation: map[int64]string{}}
		for _, tpl := range s.EmailTemplate().Query().AllX(ctx) {
			snap.templates[tpl.ID] = tpl.Body + string(tpl.Doc)
		}
		for _, b := range s.Broadcast().Query().AllX(ctx) {
			snap.broadcasts[b.ID] = b.Body
		}
		for _, a := range s.Automation().Query().AllX(ctx) {
			snap.automation[a.ID] = a.Definition
		}
		return snap
	}
	before := take()
	require.NotEmpty(t, before.broadcasts)
	require.NotEmpty(t, before.automation)

	changed := brandKitInput()
	changed.LogoUrl = ""
	changed.FooterText = ""
	saveBrandKit(t, c, fixtures.InitechPublicID, changed)

	assert.Equal(t, before, take())
}

func TestSiteBrandKitIsReadByEveryMemberAndWrittenByOwnersOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	member := env.SiteActor(t, fixtures.InitechMemberMaxEmail)
	kit := getBrandKit(t, member, fixtures.InitechPublicID)
	assert.True(t, kit.Saved)
	assert.Equal(t, "#0a7d5a", kit.AccentColor)
	assert.IsType(t, &siteapi.SiteBrandKitSaveForbidden{}, saveBrandKit(t, member, fixtures.InitechPublicID, brandKitInput()))
	res, err := member.SiteBrandKitRecreateStarterTemplates(ctx, siteapi.SiteBrandKitRecreateStarterTemplatesParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBrandKitRecreateStarterTemplatesForbidden{}, res)
	assert.Equal(t, "#0a7d5a", getBrandKit(t, member, fixtures.InitechPublicID).AccentColor, "a refused save changes nothing")

	admin := env.SiteActor(t, fixtures.InitechAdminAdaEmail)
	assert.Equal(t, "#0a7d5a", getBrandKit(t, admin, fixtures.InitechPublicID).AccentColor, "an admin reads it too")
	assert.IsType(t, &siteapi.SiteBrandKitSaveForbidden{}, saveBrandKit(t, admin, fixtures.InitechPublicID, brandKitInput()))
	res, err = admin.SiteBrandKitRecreateStarterTemplates(ctx, siteapi.SiteBrandKitRecreateStarterTemplatesParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBrandKitRecreateStarterTemplatesForbidden{}, res)
	assert.Equal(t, "#0a7d5a", getBrandKit(t, admin, fixtures.InitechPublicID).AccentColor, "a refused save changes nothing")

	owner := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	assert.IsType(t, &siteapi.SiteBrandKitResource{}, saveBrandKit(t, owner, fixtures.InitechPublicID, brandKitInput()))
}

func TestSiteBrandKitIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)

	outsider := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	res, err := outsider.SiteBrandKitGet(context.Background(), siteapi.SiteBrandKitGetParams{WorkspaceId: fixtures.VandelayPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
	assert.IsType(t, &siteapi.SiteBrandKitSaveNotFound{}, saveBrandKit(t, outsider, fixtures.VandelayPublicID, brandKitInput()))

	owner := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)
	saveBrandKit(t, owner, fixtures.VandelayPublicID, brandKitInput())
	globex := env.DB.BrandKit.GetX(context.Background(), fixtures.BrandKitGlobexID)
	assert.Equal(t, "#cc0000", globex.AccentColor, "saving Acme's kit leaves Globex's alone")
}

func TestSiteBrandKitRefusesAValueThatCannotBeWrittenIntoAMessage(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	for name, mutate := range map[string]func(*siteapi.SiteSaveBrandKitInput){
		"color is a name":       func(in *siteapi.SiteSaveBrandKitInput) { in.AccentColor = "red" },
		"color breaks out":      func(in *siteapi.SiteSaveBrandKitInput) { in.TextColor = "#fff;background:url(x)" },
		"font has a semicolon":  func(in *siteapi.SiteSaveBrandKitInput) { in.HeadingFont = "Arial; color: red" },
		"font imports":          func(in *siteapi.SiteSaveBrandKitInput) { in.BodyFont = "Arial, url(//evil)" },
		"logo is javascript":    func(in *siteapi.SiteSaveBrandKitInput) { in.LogoUrl = "javascript:alert(1)" },
		"logo has a quote":      func(in *siteapi.SiteSaveBrandKitInput) { in.LogoUrl = `https://x.test/a" onerror="b` },
		"logo is liquid":        func(in *siteapi.SiteSaveBrandKitInput) { in.LogoUrl = "https://x.test/{{ a }}" },
		"radius is huge":        func(in *siteapi.SiteSaveBrandKitInput) { in.ButtonRadius = 99 },
		"footer is liquid":      func(in *siteapi.SiteSaveBrandKitInput) { in.FooterText = "{% if %}" },
		"footer has a newline":  func(in *siteapi.SiteSaveBrandKitInput) { in.FooterText = "a\nb" },
		"footer is far too big": func(in *siteapi.SiteSaveBrandKitInput) { in.FooterText = strings.Repeat("a", 501) },
	} {
		t.Run(name, func(t *testing.T) {
			in := brandKitInput()
			mutate(in)
			assert.IsType(t, &siteapi.SiteBrandKitSaveUnprocessableEntity{}, saveBrandKit(t, c, fixtures.VandelayPublicID, in))
			assert.False(t, getBrandKit(t, c, fixtures.VandelayPublicID).Saved, "a refused save stores nothing")
		})
	}
}

func TestSiteBrandKitFooterIsWrittenAsTextNotMarkup(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.VandelayID)
	before := s.EmailTemplate().Query().CountX(ctx)

	in := brandKitInput()
	in.FooterText = `<img src=x onerror=alert(1)> & co`
	saveBrandKit(t, c, fixtures.VandelayPublicID, in)

	for _, tpl := range s.EmailTemplate().Query().AllX(ctx)[before:] {
		assert.NotContains(t, tpl.Body, "<img src=x", tpl.Name)
		assert.Contains(t, tpl.Body, "&lt;img src=x onerror=alert(1)&gt;", tpl.Name)
	}
}
