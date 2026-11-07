package tagdeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/tagdeletion"
)

func names(ts []*ent.Tag) []string {
	return lo.Map(ts, func(t *ent.Tag, _ int) string { return t.Name })
}

// Deleting a Tag runs its check and the delete in one transaction under the Workspace's lock, and
// saving a Segment takes the same lock (ADR 0099): a Segment saved first, naming the Tag, is what
// the delete then finds, so the Tag is kept instead of vanishing under it.
func TestDeletingATagWaitsForASegmentSavedNamingIt(t *testing.T) {
	ctx := context.Background()
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	s := race.Scope
	segs, catalogue, deletion := segments.New(race.Bus, time.Now), tags.New(), tagdeletion.New(race.Bus, removalcheck.New(messaging.NewResolver(testhelper.Cipher(t), registry.Default())))

	var saved int64
	save := testhelper.Writer{Name: "save", Run: func() error {
		cond := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"has","value":"` + fixtures.TagUnusedName + `"}]}`
		seg, err := segs.Create(ctx, s, segments.CreateInput{Name: "Unused tag holders", Condition: new(cond)})
		if seg != nil {
			saved = seg.ID
		}
		return err
	}}
	remove := testhelper.Writer{Name: "delete", Run: func() error { return deletion.Delete(ctx, s, fixtures.TagUnusedID) }}

	errs := race.Queue(t, save, remove)

	require.NoError(t, errs["save"])
	var used *liveuse.Error
	require.ErrorAs(t, errs["delete"], &used)
	assert.Equal(t, []liveuse.User{{Kind: liveuse.Segment, ID: saved, Name: "Unused tag holders"}}, used.Users)
	kept, err := catalogue.List(ctx, s)
	require.NoError(t, err)
	assert.Contains(t, names(kept), fixtures.TagUnusedName, "the Tag the Segment names is kept")
}
