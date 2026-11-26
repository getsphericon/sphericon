package tagdeletion_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/segmentsaving"
	"github.com/getsphericon/sphericon/internal/usecase/tagdeletion"
)

func names(ts []*ent.Tag) []string {
	return lo.Map(ts, func(t *ent.Tag, _ int) string { return t.Name })
}

// Deleting a Tag runs its check and the delete in one transaction under the Workspace's lock, and
// saving a Segment takes the same lock (ADR 0099): a Segment saved first, naming the Tag, is what
// the delete then finds, so the Tag is kept instead of vanishing under it.
func TestDeletingATagWaitsForASegmentSavedNamingIt(t *testing.T) {
	ctx := t.Context()
	env := testhelper.Setup(t, testhelper.Committed())
	scope := env.DB.Scoped(fixtures.AcmeID)
	s := scope
	saving, catalogue, deletion := testhelper.Module[*segmentsaving.UseCase](env), tags.New(), testhelper.Module[*tagdeletion.UseCase](env)

	var saved int64
	save := testhelper.Writer{Name: "save", Run: func() error {
		cond := `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"` + fixtures.TagUnusedName + `","operator":"has"}]}`
		seg, err := saving.Create(ctx, s, segments.CreateInput{Name: "Unused tag holders", Condition: new(testhelper.SegmentCondition(t, cond))})
		if seg != nil {
			saved = seg.ID
		}
		return err
	}}
	remove := testhelper.Writer{Name: "delete", Run: func() error { return deletion.Delete(ctx, s, fixtures.TagUnusedID) }}

	errs := env.QueueBehindWorkspaceLock(t, scope, save, remove)

	require.NoError(t, errs["save"])
	var used *liveuse.Error
	require.ErrorAs(t, errs["delete"], &used)
	assert.Equal(t, []liveuse.User{{Kind: liveuse.Segment, ID: saved, Name: "Unused tag holders"}}, used.Users)
	kept, err := catalogue.List(ctx, s)
	require.NoError(t, err)
	assert.Contains(t, names(kept), fixtures.TagUnusedName, "the Tag the Segment names is kept")
}
