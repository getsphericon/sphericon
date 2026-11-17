// Package capturedmessagesunchecked holds the Captured message part of a Contact's Erasure. It is a
// step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use
// case internal/usecase/contacterasure runs each owner's step in its order, in the one transaction
// of the Erasure; only a use case may import this one (ADR 0110). The capturedmessages module does
// not import it.
package capturedmessagesunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
)

// Module holds the Captured message part of a Contact's Erasure; the use case that runs it builds
// its own in its constructor (docs/architecture/modules.md, the unchecked operations).
type Module struct{}

// New builds the unchecked operations; they need nothing but the scope each call is given.
func New() *Module { return &Module{} }

// EraseTo is the Captured message part of a Contact's Erasure (ADR 0021): it deletes the Captured
// messages addressed to the subject's addresses. They hold the rendered mail, so unlike a delivery
// record there is nothing to keep. Those kept by an archived Test inbox go too. Run it inside the
// Erasure's transaction.
func (m *Module) EraseTo(ctx context.Context, s *ent.Scoped, destinations []string) error {
	if len(destinations) == 0 {
		return nil
	}
	_, err := s.CapturedMessage().Delete().Where(capturedmessage.RecipientIn(destinations...)).Exec(ctx)
	return err
}
