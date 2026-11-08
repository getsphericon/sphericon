package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventreceipt"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// Subscriber applies a mandatory reaction in the publishing transaction. Its filter is pure;
// its handler calls the owners of the affected rows through the same transaction scope.
type Subscriber struct {
	Wants  func(Envelope, DomainEvent) bool
	Handle func(context.Context, *ent.Scoped, Publisher, Envelope, DomainEvent) error
}

// UseSubscribers registers synchronous reactions at composition time, before publication starts.
func (b *Bus) UseSubscribers(subscribers ...Subscriber) {
	b.subscribers = append(b.subscribers, subscribers...)
}

// claimExternal remembers only the digest of the fact's identity. It shares the transaction
// with subscriber writes, so a failed attempt does not consume the fact.
func (p *txPublisher) claimExternal(ctx context.Context, env Envelope) (bool, error) {
	if env.DedupKey == "" {
		return true, nil
	}
	identity, err := json.Marshal([]any{env.WorkspaceID, env.Name, env.DedupKey})
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(identity)
	key := hex.EncodeToString(digest[:])
	err = p.client.EventReceipt.Create().SetReceiptKey(key).
		OnConflictColumns(eventreceipt.FieldReceiptKey).DoNothing().Exec(ctx)
	claimed, err := db.Inserted(err)
	if err != nil {
		return false, fmt.Errorf("claim external fact: %w", err)
	}
	return claimed, nil
}

func (p *txPublisher) react(ctx context.Context, env Envelope, ev DomainEvent) error {
	ctx = Handling(ctx, env)
	for _, subscriber := range p.subscribers {
		if subscriber.Wants(env, ev) {
			s := p.client.Scoped(env.WorkspaceID)
			if err := subscriber.Handle(ctx, s, p, env, ev); err != nil {
				return fmt.Errorf("react to %q: %w", env.Name, err)
			}
		}
	}
	return nil
}
