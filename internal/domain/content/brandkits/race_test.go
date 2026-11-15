package brandkits_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var kit = brandkits.Input{
	Revision:    1,
	AccentColor: "#ff5500", TextColor: "#102030", BackgroundColor: "#f0f4f8",
	HeadingFont: "Georgia, serif", BodyFont: "Verdana, sans-serif", LogoURL: "https://cdn.example.com/logo.png",
	ButtonTextColor: "#fefefe", ButtonRadius: 12, FooterText: "Umbrella, 1 Main St",
}

// A first save that a concurrent first save beat to the row finds the Brand kit there when its
// insert goes ahead (ON CONFLICT DO NOTHING), saved since the revision it was made on (ADR 0119): it
// is refused rather than replacing the first save's values, and makes no starter Templates, which
// the save that created the Brand kit made.
func TestAFirstSaveThatLosesTheRaceIsRefusedAndMakesNoStarters(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	scope := env.DB.Scoped(fixtures.UmbrellaID)
	ctx := context.Background()
	templates := func() int {
		n, err := scope.EmailTemplate().Query().Count(ctx)
		require.NoError(t, err)
		return n
	}
	before := templates()

	release := env.HoldOpen(t, events.Ingest(scope), func(ts *ent.Scoped, _ events.Publisher) error {
		// test writes: the Brand kit of the concurrent first save, held uncommitted while the other runs.
		return ts.BrandKit().Create().
			SetAccentColor("#000000").SetTextColor(kit.TextColor).SetBackgroundColor(kit.BackgroundColor).
			SetHeadingFont(kit.HeadingFont).SetBodyFont(kit.BodyFont).SetLogoURL(kit.LogoURL).
			SetButtonTextColor(kit.ButtonTextColor).SetButtonRadius(kit.ButtonRadius).SetFooterText(kit.FooterText).
			SetRevision(2).
			Exec(ctx)
	})

	module := testhelper.Module[*brandkits.Module](env)
	errs := env.QueueBehind(t, release,
		testhelper.Writer{Name: "second first save", Run: func() error {
			_, err := module.Save(ctx, events.Ingest(scope), kit)
			return err
		}})

	require.ErrorIs(t, errs["second first save"], brandkits.ErrRevisionChanged)
	stored, err := scope.BrandKit().Query().Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "#000000", stored.AccentColor, "the first save's values stay")
	after := templates()
	assert.Equal(t, before, after, "the losing save makes no starter Templates")
}
