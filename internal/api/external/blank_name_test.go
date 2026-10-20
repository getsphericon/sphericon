package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A name the schema's validators refuse is the client's to fix: 422, never a server error (found by
// fuzzing the contract, mise run check:contract).
func TestExternalBlankNamesAreRefusedWith422(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:write", "broadcasts:write")
	const def = `{"kind":"group","combinator":"and","rules":[]}`

	segment, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "", Definition: testhelper.ExternalRuleGroup(t, def)})
	require.NoError(t, err)
	broadcast, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString("")},
		externalapi.BroadcastsUpdateParams{ID: draftBroadcast})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.SegmentsCreateUnprocessableEntity{}, segment)
	assert.IsType(t, &externalapi.BroadcastsUpdateUnprocessableEntity{}, broadcast)
}
