package automations

import (
	"context"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// State is where an Automation is in its life, as the list shows it: a draft until it is first
// published, then active or inactive (docs/standards/ui.md, "Status and its details").
type State string

const (
	// StateDraft is an Automation never published: it has no live version.
	StateDraft State = "draft"
	// StateActive is a published Automation that enrolls Contacts.
	StateActive State = "active"
	// StateInactive is a published Automation that enrolls nobody.
	StateInactive State = "inactive"
)

// Sort is what the list is ordered by.
type Sort string

const (
	// SortCreatedAt orders by creation, the default.
	SortCreatedAt Sort = "createdAt"
	// SortName orders by name.
	SortName Sort = "name"
)

// ListFilter narrows and orders the Automation list; the zero value is the kept Automations, the
// newest first.
type ListFilter struct {
	// Archived lists the archived Automations instead of the kept ones.
	Archived bool
	// Name keeps the Automations whose name contains it, ignoring case.
	Name string
	// State keeps the Automations in it; empty keeps every one.
	State State
	// Sort is what the list is ordered by; empty is SortCreatedAt.
	Sort Sort
	// Direction of the order; empty is the newest first by creation, A to Z by name.
	Direction pagination.Direction
}

// where is the predicate of the State; none keeps every Automation.
func (s State) where() []predicate.Automation {
	switch s {
	case StateDraft:
		return []predicate.Automation{automation.LiveVersionIDIsNil()}
	case StateActive:
		return []predicate.Automation{automation.StatusEQ(automation.StatusActive)}
	case StateInactive:
		return []predicate.Automation{automation.StatusEQ(automation.StatusInactive), automation.LiveVersionIDNotNil()}
	}
	return nil
}

// terms is the ORDER BY of the filter. The id breaks every tie in the same direction, so a page is
// stable and the creation order is the id's.
func (f ListFilter) terms() []automation.OrderOption {
	dir := f.Direction
	if dir == "" {
		dir = pagination.Desc
		if f.Sort == SortName {
			dir = pagination.Asc
		}
	}
	term := sql.OrderAsc()
	if dir == pagination.Desc {
		term = sql.OrderDesc()
	}
	id := automation.ByID(term)
	if f.Sort == SortName {
		return []automation.OrderOption{automation.ByName(term), id}
	}
	return []automation.OrderOption{id}
}

// List returns one page of the Workspace's Automations that f lets through, in its order: the kept
// ones, or with Archived the archived ones.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, f ListFilter) (pagination.Page[*ent.Automation], error) {
	rows := func() *ent.AutomationQuery {
		q := s.Automation().Query()
		if f.Archived {
			q = s.Automation().QueryArchived()
		}
		if f.Name != "" {
			q = q.Where(automation.NameContainsFold(f.Name))
		}
		return q.Where(f.State.where()...)
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Automation, error) {
			return withVersions(rows()).Order(f.terms()...).Limit(limit).Offset(offset).All(ctx)
		})
}
