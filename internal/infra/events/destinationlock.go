package events

import (
	"context"
	"fmt"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
)

// WithinDestinationLockTx is WithinScopedTx holding the locks of the Destinations a message is
// claimed for (ADR 0178): its first statements take the transaction-scoped advisory lock of each of
// keys (postgres.DestinationKey), in ascending order, held until the transaction ends, and fn gets
// the scope that stands for them, *ent.DestinationLocked, which the writer of a claim and its
// Frequency cap count takes. Two sends to one Destination count and claim one after the other, and
// the one that waits reads what the other committed.
//
// The keys are the caller's to choose: no keys take no lock, and fn still gets the type, so the code
// has one path. The locks come before any row lock the transaction takes and are taken in one order,
// so two of these never wait for each other in a cycle; this bus is the one caller of
// ent.Scoped.LockDestinations (forbidigo). The transaction holds no call to a provider: the locks are
// held from the count to the claim's commit only.
func (b *Bus) WithinDestinationLockTx(ctx context.Context, s *ent.Scoped, keys []int64, fn func(l *ent.DestinationLocked) error) error {
	ordered := slices.Compact(slices.Sorted(slices.Values(keys)))
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error {
		l, err := s.InTx(tx, auditPublisher{pub: pub}).LockDestinations(ctx, func(ctx context.Context, tx *ent.Client, _ int64) error {
			return lockDestinations(ctx, tx, ordered)
		})
		if err != nil {
			return err
		}
		return fn(l)
	})
}

// lockDestinations takes the lock of each of keys on the transaction tx, in their order.
func lockDestinations(ctx context.Context, tx *ent.Client, keys []int64) error {
	for _, key := range keys {
		if err := postgres.LockTx(ctx, tx, key); err != nil {
			return fmt.Errorf("lock a Destination: %w", err)
		}
	}
	return nil
}
