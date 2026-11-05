// Package staff stands in for ee/api/staff: the handlers of the Enterprise Staff and service APIs, an
// API surface like the core ones.
package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

type Handlers struct{ client *ent.Client }

// A handler writes through ent: a finding.
func (h *Handlers) save(ctx context.Context) error {
	return h.client.Tag.Create().Save(ctx) // want `^rule 5: an entry point writes through ent \(Create on Tag\).*; fix: move the write into the domain module that owns Tag`
}

// A handler reads through ent: a finding, as on every surface.
func (h *Handlers) list(ctx context.Context) error {
	_, err := h.client.Tag.Query().All(ctx) // want `^rule 5: a handler reads through ent \(Query on Tag\)`
	return err
}
