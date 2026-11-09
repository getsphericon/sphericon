package brandkits_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/content/emailtemplates"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var kit = brandkits.Input{
	AccentColor: "#ff5500", TextColor: "#102030", BackgroundColor: "#f0f4f8",
	HeadingFont: "Georgia, serif", BodyFont: "Verdana, sans-serif", LogoURL: "https://cdn.example.com/logo.png",
	ButtonTextColor: "#fefefe", ButtonRadius: 12, FooterText: "Umbrella, 1 Main St",
}

// A first save that a concurrent first save beat to the row finds the Brand kit there when its
// insert goes ahead (ON CONFLICT DO NOTHING): it replaces the values and makes no starter Templates,
// which the save that created the Brand kit made.
func TestAFirstSaveThatLosesTheRaceUpdatesAndMakesNoStarters(t *testing.T) {
	race := testhelper.NewWorkspaceRace(t, fixtures.UmbrellaID)
	ctx := context.Background()
	templates := func() int {
		n, err := race.Scope.EmailTemplate().Query().Count(ctx)
		require.NoError(t, err)
		return n
	}
	before := templates()

	release := race.HoldOpen(t, events.Ingest(race.Scope), func(ts *ent.Scoped, _ events.Publisher) error {
		// test writes: the Brand kit of the concurrent first save, held uncommitted while the other runs.
		return ts.BrandKit().Create().
			SetAccentColor("#000000").SetTextColor(kit.TextColor).SetBackgroundColor(kit.BackgroundColor).
			SetHeadingFont(kit.HeadingFont).SetBodyFont(kit.BodyFont).SetLogoURL(kit.LogoURL).
			SetButtonTextColor(kit.ButtonTextColor).SetButtonRadius(kit.ButtonRadius).SetFooterText(kit.FooterText).
			Exec(ctx)
	})

	module := brandkits.New(race.Bus, emailtemplates.New(testhelper.Renderer(), brandkits.Brands{}))
	var saved brandkits.Kit
	errs := testhelper.QueuedBehind(t, race.DB, release,
		testhelper.Writer{Name: "second first save", Run: func() error {
			var err error
			saved, err = module.Save(ctx, events.Ingest(race.Scope), kit)
			return err
		}})

	require.NoError(t, errs["second first save"])
	assert.Equal(t, kit.AccentColor, saved.AccentColor, "the losing save replaces the values")
	n, err := race.Scope.BrandKit().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	after := templates()
	assert.Equal(t, before, after, "the losing save makes no starter Templates")
}
