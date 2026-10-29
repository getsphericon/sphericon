package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// passkeyChallengesHourUTC is the off-peak hour the spent-challenge prune runs at.
const passkeyChallengesHourUTC = 5

// PrunePasskeyChallengesArgs is the daily tick that deletes spent Passkey challenges (ADR 0051).
type PrunePasskeyChallengesArgs struct{}

func (PrunePasskeyChallengesArgs) Kind() string { return "passkey_challenges_prune" }

// PrunePasskeyChallengesWorker deletes the spent challenges older than
// secondfactor.SpentChallengeRetention, which outlasts any ceremony cookie. A challenge
// belongs to no User, so it works on the raw client.
type PrunePasskeyChallengesWorker struct {
	river.WorkerDefaults[PrunePasskeyChallengesArgs]

	ent *ent.Client
	now func() time.Time
}

// NewPrunePasskeyChallengesWorker builds the prune of spent Passkey challenges, aged by the clock now.
func NewPrunePasskeyChallengesWorker(client *ent.Client, now func() time.Time) *PrunePasskeyChallengesWorker {
	return &PrunePasskeyChallengesWorker{ent: client, now: now}
}

func (w *PrunePasskeyChallengesWorker) Work(ctx context.Context, _ *river.Job[PrunePasskeyChallengesArgs]) error {
	deleted, err := secondfactor.PruneSpentChallenges(ctx, w.ent, w.now().Add(-secondfactor.SpentChallengeRetention))
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "spent passkey challenges pruned", "deleted", deleted)
	}
	return nil
}
