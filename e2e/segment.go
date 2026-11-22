//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// Segment is a Segment the console saved, as a later step names it.
type Segment struct {
	ID siteapi.EntityId
}

// SaveSegment is a person building a Segment in the console: its Condition is the typed tree of a
// Segment (ADR 0181), written as JSON, and it reads back as saved.
func (w *Workspace) SaveSegment(condition string) Segment {
	w.t.Helper()
	res, err := w.site.SiteSegmentsCreate(w.t.Context(), &siteapi.CreateSegmentInput{
		Name:      siteapi.Name("e2e segment " + uniq()),
		Condition: conditionOf[siteapi.SegmentConditionInput](w.t, condition),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)})
	saved := ok[siteapi.SiteSegmentResource](w.t, "create segment", res, err)
	requireCondition(w.t, condition, &saved.Condition)
	return Segment{ID: saved.ID}
}

// EditSegment is a person changing a Segment's Condition in the console, on the revision they read.
func (w *Workspace) EditSegment(s Segment, condition string) {
	w.t.Helper()
	ctx := w.t.Context()
	ws := siteapi.WorkspaceId(w.publicID)
	read, err := w.site.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: ws, ID: s.ID})
	current := ok[siteapi.SiteSegmentResource](w.t, "read segment", read, err)
	res, err := w.site.SiteSegmentsUpdate(ctx, &siteapi.UpdateSegmentInput{
		Revision:  current.Revision,
		Condition: siteapi.NewOptSegmentConditionInput(conditionOf[siteapi.SegmentConditionInput](w.t, condition)),
	}, siteapi.SiteSegmentsUpdateParams{WorkspaceId: ws, ID: s.ID})
	saved := ok[siteapi.SiteSegmentResource](w.t, "edit segment", res, err)
	requireCondition(w.t, condition, &saved.Condition)
}

// conditionOf is a Condition written as JSON, as the console's builder sends it.
func conditionOf[C any](tb testing.TB, condition string) C {
	tb.Helper()
	var c C
	require.NoError(tb, json.Unmarshal([]byte(condition), &c), "the Condition is a tree of its place")
	return c
}

// requireCondition checks that a Condition reads back as it was sent. The generated type encodes
// itself: encoding/json would call an unset optional field's own encoder and fail.
func requireCondition(tb testing.TB, sent string, saved json.Marshaler) {
	tb.Helper()
	got, err := saved.MarshalJSON()
	require.NoError(tb, err)
	require.JSONEq(tb, sent, string(got), "the Condition reads back as saved")
}
