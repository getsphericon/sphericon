package site_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A stored Condition is checked when it is written, so one the contract cannot read is a broken
// row: the read fails with a server error, never answering the Segment without its Condition.
func TestASegmentWhoseStoredConditionDoesNotDecodeFailsTheRead(t *testing.T) {
	env := testhelper.Setup(t)
	var broken conditiontree.Segment
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","entries":[{"kind":"nope"}]}`), &broken))
	// test writes: a row no write path makes, which only the database's CHECK of entries lets by.
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Segment().UpdateOneID(fixtures.SegmentActiveID).SetCondition(broken).Exec(t.Context()))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSegmentsGet(t.Context(), siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentActiveID)})

	failed, ok := errors.AsType[*siteapi.ProblemDetailsStatusCode](err)
	require.True(t, ok, "a problem: %v", err)
	require.Equal(t, http.StatusInternalServerError, failed.StatusCode)
}
