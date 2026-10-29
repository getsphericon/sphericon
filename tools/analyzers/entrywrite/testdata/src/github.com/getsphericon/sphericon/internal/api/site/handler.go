package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
)

type tags interface {
	Get(ctx context.Context, s *ent.Scoped, id int64) error
	Create(ctx context.Context, s *ent.Scoped, name string) error
}

// A handler calls a module for every read and every write; it may name the scope's Workspace id.
func handle(ctx context.Context, s *ent.Scoped, m tags) error {
	if err := m.Get(ctx, s, s.WorkspaceID()); err != nil {
		return err
	}
	return m.Create(ctx, s, "x")
}

// A handler that reads through ent itself is a finding at each read it starts.
func reads(ctx context.Context, s *ent.Scoped, t *ent.Tag) error {
	if _, err := s.Tag().Query().All(ctx); err != nil { // want `^rule 5: a handler reads through ent \(Query on Tag\).*; fix: give the domain module`
		return err
	}
	if err := s.Tag().Get(ctx, 1); err != nil { // want `^rule 5: .*\(Get on Tag\)`
		return err
	}
	if _, err := s.Workspace(ctx); err != nil { // want `^rule 5: .*\(Workspace on Workspace\)`
		return err
	}
	_, err := t.QueryContacts().All(ctx) // want `^rule 5: .*\(QueryContacts on Tag\)`
	return err
}

// A handler that opens a transaction is a finding.
func tx(ctx context.Context, s *ent.Scoped, bus *events.Bus) error {
	return bus.WithinScopedTx(ctx, s, func(*ent.Scoped) error { return nil }) // want `^rule 5: a handler opens a transaction \(WithinScopedTx on the events bus\)`
}
