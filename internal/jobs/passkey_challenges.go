package jobs

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
)

// PrunePasskeyChallengesArgs is the daily tick that deletes spent Passkey challenges (ADR 0051).
type PrunePasskeyChallengesArgs struct{}

func (PrunePasskeyChallengesArgs) Kind() string { return "passkey_challenges_prune" }

// PrunePasskeyChallengesWorker deletes the spent challenges older than
// passkeys.SpentChallengeRetention, which outlasts any ceremony cookie.
type PrunePasskeyChallengesWorker struct {
	river.WorkerDefaults[PrunePasskeyChallengesArgs]

	keys *passkeys.Module
}

// NewPrunePasskeyChallengesWorker builds the prune of spent Passkey challenges, aged by the clock of
// the Passkeys module.
func NewPrunePasskeyChallengesWorker(keys *passkeys.Module) *PrunePasskeyChallengesWorker {
	return &PrunePasskeyChallengesWorker{keys: keys}
}

func (w *PrunePasskeyChallengesWorker) Work(ctx context.Context, _ *river.Job[PrunePasskeyChallengesArgs]) error {
	deleted, err := w.keys.PruneSpentChallenges(ctx)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "spent passkey challenges pruned", "deleted", deleted)
	}
	return nil
}
