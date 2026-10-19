package site_test

import (
	"context"
	"errors"
	"testing"

	entgo "entgo.io/ent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The first save is one transaction: a starter Template that cannot be written takes the Brand
// kit and the Templates already made with it.
func TestSiteBrandKitFirstSaveIsAtomicWithItsStarterTemplates(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.VandelayID)
	before := s.EmailTemplate().Query().CountX(ctx)
	made := 0
	env.Bus.Use(func(next entgo.Mutator) entgo.Mutator {
		return entgo.MutateFunc(func(ctx context.Context, m entgo.Mutation) (entgo.Value, error) {
			if m.Type() == ent.TypeEmailTemplate {
				made++
				if made == 3 {
					return nil, errors.New("injected failure")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	c := env.SiteActor(t, fixtures.VandelayOwnerArtEmail)

	_, err := c.SiteBrandKitSave(ctx, brandKitInput(), siteapi.SiteBrandKitSaveParams{Slug: fixtures.VandelaySlug})
	require.Error(t, err)

	assert.False(t, getBrandKit(t, c, fixtures.VandelaySlug).Saved, "no Brand kit without its starters")
	assert.Equal(t, before, s.EmailTemplate().Query().CountX(ctx), "no starter Template without the Brand kit")
}
