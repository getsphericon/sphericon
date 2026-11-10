package integrations

import (
	"context"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// View is an Integration as its Workspace's members and agents read it, built once for every
// surface (ADR 0111): the row, the provider config and the params without a secret, the hook URL
// its provider posts callbacks to and its Provider limit with the last 24 hours of sends. Each
// surface maps it to its own resource.
type View struct {
	Row *ent.Integration
	// Config is the provider config with its secrets left out.
	Config RedactedConfig
	// Params are the params as saved, each secret masked, as the opaque values of a response.
	Params map[string]jx.Raw
	// SecretsRetiring names the secrets of the params that are rotating.
	SecretsRetiring []string
	// HookURL is where the provider posts the Integration's callbacks.
	HookURL string
	// ProviderLimit is the Provider limit as enforced, with the messages sent over the last 24
	// hours.
	ProviderLimit providerlimit.Status
}

// View reads the Integration of the id as a View. An unknown id is ErrNotFound.
func (m *Module) View(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	row, err := m.Get(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.viewOf(ctx, s, row)
}

// All reads every kept Integration of the Workspace as a View, ascending by id.
func (m *Module) All(ctx context.Context, s *ent.Scoped) ([]View, error) {
	rows, err := s.Integration().Query().Order(ent.Asc(integration.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	return m.views(ctx, s, rows)
}

// List reads one page of the Workspace's kept Integrations as Views, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[View], error) {
	page, err := pagination.List(ctx, p, s.Integration().Query().Order(ent.Asc(integration.FieldID)))
	if err != nil {
		return pagination.Page[View]{}, err
	}
	views, err := m.views(ctx, s, page.Items)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	return pagination.Page[View]{
		Items: views, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages,
	}, nil
}

// viewOf is the View of one row, the answer of a read or of a write of it.
func (m *Module) viewOf(ctx context.Context, s *ent.Scoped, row *ent.Integration) (View, error) {
	views, err := m.views(ctx, s, []*ent.Integration{row})
	if err != nil {
		return View{}, err
	}
	return views[0], nil
}

// views builds the View of each row, reading the Workspace's 24-hour usage once for all of them.
func (m *Module) views(ctx context.Context, s *ent.Scoped, rows []*ent.Integration) ([]View, error) {
	limits, err := providerlimit.Statuses(ctx, s, rows, m.now())
	if err != nil {
		return nil, err
	}
	out := make([]View, len(rows))
	for i, row := range rows {
		cfg, err := m.Redact(row)
		if err != nil {
			return nil, err
		}
		params, err := m.paramsView(row)
		if err != nil {
			return nil, err
		}
		out[i] = View{
			Row:             row,
			Config:          cfg,
			Params:          RawOf(params.Values),
			SecretsRetiring: params.SecretsRetiring,
			HookURL:         m.HookURL(row),
			ProviderLimit:   limits[row.ID],
		}
	}
	return out, nil
}
