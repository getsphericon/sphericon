// Package conditionspike is a SPIKE (#1674, ADR 0181), never merged: it stores a Condition of one
// place as the typed tree's JSON and reads it back, the stand-in for the stored shape #1676 decides.
package conditionspike

import (
	"context"
	"encoding/json"

	"github.com/getsphericon/sphericon/ent"
)

// Place is whose tree a Condition is.
type Place string

const (
	PlaceSegment Place = "segment"
	PlaceRule    Place = "rule"
)

// Input is a Condition a request sends, as its typed tree's JSON.
type Input struct {
	Condition json.RawMessage
}

// Store writes the Condition and returns the row read back from the database.
func Store(ctx context.Context, ws *ent.Scoped, place Place, in Input) (*ent.ConditionSpike, error) {
	row, err := ws.ConditionSpike().Create().
		SetPlace(string(place)).
		SetCondition(in.Condition).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return ws.ConditionSpike().Get(ctx, row.ID)
}
