package testhelper

import (
	"context"
	"time"
)

// ExtendHold moves the expiry of a fixture Staff hold of a Workspace to `until`: the fixture
// holds are lapsed, so a test that wants a freeze lifts one of them instead of creating a hold.
func (env *TestEnv) ExtendHold(ctx context.Context, workspaceID, holdID int64, until time.Time) {
	if err := env.DB.Scoped(workspaceID).Hold().UpdateOneID(holdID).SetExpiresAt(until).Exec(ctx); err != nil {
		panic(err)
	}
}
