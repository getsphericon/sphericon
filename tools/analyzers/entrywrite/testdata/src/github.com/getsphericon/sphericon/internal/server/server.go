package server

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The router is the wiring, no entry point: a write there is any non-owner's write.
func mount(ctx context.Context, s *ent.Scoped) error {
	return s.Tag().Create().Save(ctx) // want `^rule 5: internal/server writes Tag \(Create on Tag\), a table internal/domain/contacts/tags owns`
}
