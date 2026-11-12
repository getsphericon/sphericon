package external_test

import (
	"context"
	"net/http"
	"testing"

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

	_, segment := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "", Condition: testhelper.ExternalRuleGroup(t, def)})
	_, broadcast := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), Name: externalapi.NewOptName("")},
		externalapi.BroadcastsUpdateParams{ID: draftBroadcast})

	testhelper.ExternalRefusal(t, segment, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, broadcast, http.StatusUnprocessableEntity)
}
