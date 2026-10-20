package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	entgo "entgo.io/ent"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	entworkspace "github.com/getsphericon/sphericon/ent/workspace"
)

// Contacts is the peak number of identified Contacts reached during the Usage period:
// those with an email Destination, never an instantaneous value. An erased Contact is
// gone from the table and an anonymous tracker Contact has no email, so neither counts.
const Contacts Metric = "contacts"

// identified selects the Contacts that count: those with an email Destination.
func identified() predicate.Contact {
	return contact.And(contact.EmailNotNil(), contact.EmailNEQ(""))
}

func countIdentified(ctx context.Context, s *ent.Scoped) (int64, error) {
	n, err := s.Contact().Query().Where(identified()).Count(ctx)
	return int64(n), err
}

// measureContacts is the peak of the open period: what Hook already stored, or the
// Contacts there are now when it stored nothing yet. A closed period keeps the peak
// stored while it was open; when none was stored (a Workspace nobody touched or the job
// never saw in that period) only a lower bound is left, the identified Contacts that
// existed before the period ended and still do.
func measureContacts(ctx context.Context, s *ent.Scoped, p Period, open bool, prior *ent.Usage) (int64, error) {
	if !open {
		if prior != nil {
			return prior.Value, nil
		}
		n, err := s.Contact().Query().Where(identified(), contact.CreatedAtLT(p.End)).Count(ctx)
		return int64(n), err
	}
	n, err := countIdentified(ctx, s)
	if err != nil {
		return 0, err
	}
	if prior != nil {
		n = max(n, prior.Value)
	}
	return n, nil
}

// Hook is the ent hook that keeps the contacts peak as Contacts change, because a peak
// cannot be rebuilt afterwards. It is installed on every client (the long-lived one and
// the event bus's transaction client) by the Edition. Without the `staff` license it
// is not installed at all: the returned hook hands every mutation straight on.
//
// A mutation that can only raise the count (a create or an email set) raises the open
// period's peak to the count afterwards; one that lowers it (a delete, an email
// cleared) first records the count before it. Both run in the mutation's own
// transaction, so the peak commits with the change it describes.
func Hook(lic *licensekey.License, now func() time.Time) entgo.Hook {
	return func(next entgo.Mutator) entgo.Mutator {
		if !lic.Has(licensekey.FeatureStaff) {
			return next
		}
		return entgo.MutateFunc(func(ctx context.Context, m entgo.Mutation) (entgo.Value, error) {
			mu, ok := m.(*ent.ContactMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			before, after, err := affected(ctx, mu)
			if err != nil {
				return nil, err
			}
			if err := raise(ctx, mu.Client(), now(), before...); err != nil {
				return nil, err
			}
			v, err := next.Mutate(ctx, m)
			if err != nil {
				return v, err
			}
			return v, raise(ctx, mu.Client(), now(), after...)
		})
	}
}

// affected is the Workspaces whose peak the mutation must record before it runs
// (it can lower the count) and after it ran (it can raise it).
func affected(ctx context.Context, mu *ent.ContactMutation) (before, after []int64, err error) {
	switch {
	case mu.Op().Is(entgo.OpCreate):
		if email, ok := mu.Email(); ok && email != "" {
			if ws, ok := mu.WorkspaceID(); ok {
				after = []int64{ws}
			}
		}
	case mu.Op().Is(entgo.OpDelete | entgo.OpDeleteOne):
		before, err = workspacesOf(ctx, mu, true)
	case mu.Op().Is(entgo.OpUpdate | entgo.OpUpdateOne):
		email, sets := mu.Email()
		if mu.EmailCleared() || (sets && email == "") {
			before, err = workspacesOf(ctx, mu, true)
		} else if sets {
			after, err = workspacesOf(ctx, mu, false)
		}
	}
	return before, after, err
}

// workspacesOf is the Workspaces of the Contacts the mutation targets; identifiedOnly
// narrows them to the Contacts that count now.
func workspacesOf(ctx context.Context, mu *ent.ContactMutation, identifiedOnly bool) ([]int64, error) {
	ids, err := mu.IDs(ctx)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	q := mu.Client().Contact.Query().Where(contact.IDIn(ids...))
	if identifiedOnly {
		q = q.Where(identified())
	}
	rows, err := q.Select(contact.FieldWorkspaceID).All(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]bool, len(rows))
	var out []int64
	for _, r := range rows {
		if !seen[r.WorkspaceID] {
			seen[r.WorkspaceID] = true
			out = append(out, r.WorkspaceID)
		}
	}
	return out, nil
}

// raise lifts the open period's contacts peak of each Workspace to the number of
// identified Contacts it has now, never lowering it and never touching a closed row.
func raise(ctx context.Context, c *ent.Client, now time.Time, workspaceIDs ...int64) error {
	period := PeriodOf(now)
	for _, id := range workspaceIDs {
		// A Workspace in its deletion window, or erased, is no longer measured.
		live, err := c.Workspace.Query().Where(entworkspace.ID(id), entworkspace.DiscardedAtIsNil()).Exist(ctx)
		if err != nil {
			return fmt.Errorf("load workspace: %w", err)
		}
		if !live {
			continue
		}
		s := c.Scoped(id)
		n, err := countIdentified(ctx, s)
		if err != nil {
			return fmt.Errorf("count contacts: %w", err)
		}
		// A growing import finds the row and raises it: one statement. Only when nothing
		// was raised (the row is absent, or already at least n) is the row inserted.
		raised, err := s.Usage().Update().
			Where(entusage.PeriodStart(period), entusage.Metric(string(Contacts)), entusage.Final(false), entusage.ValueLT(n)).
			SetValue(n).SetComputedAt(now).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("raise contacts peak: %w", err)
		}
		if raised > 0 {
			continue
		}
		err = s.Usage().Create().
			SetPeriodStart(period).SetMetric(string(Contacts)).
			SetValue(n).SetComputedAt(now).
			OnConflictColumns(entusage.FieldWorkspaceID, entusage.FieldPeriodStart, entusage.FieldMetric).
			DoNothing().Exec(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("record contacts peak: %w", err)
		}
	}
	return nil
}
