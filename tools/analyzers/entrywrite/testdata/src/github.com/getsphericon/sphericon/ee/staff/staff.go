package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

type Handlers struct{ client *ent.Client }

type Module struct{ client *ent.Client }

// A handler writes through ent: it is an entry point.
func (h *Handlers) save(ctx context.Context) error {
	return h.client.Tag.Create().Save(ctx) // want `^rule 5: an entry point writes through ent \(Create on Tag\).*; fix: move the write into the domain module that owns Tag`
}

// The domain module of the package writes through ent: it is not an entry point.
func (m *Module) save(ctx context.Context) error {
	return m.client.Tag.Create().Save(ctx)
}
