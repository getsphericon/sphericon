package segments_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// UsingTag names the Segments whose rule names the Tag, archived ones too: restoring one must
// not leave it naming a missing Tag.
func TestUsingTagIncludesArchivedSegments(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	require.NoError(t, acme.Segment().Discard(ctx, fixtures.SegmentNewsletterReadersID))

	using, err := segments.UsingTag(ctx, acme, "newsletter")
	require.NoError(t, err)
	names := lo.Map(using, func(s *ent.Segment, _ int) string { return s.Name })
	assert.Contains(t, names, fixtures.SegmentNewsletterReadersName)
}
