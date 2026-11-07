// Package segmentsuse reads the Definition use the Segments make of a row (GLOSSARY "Definition
// use"): the Segments whose Condition names a Tag or tests a Custom field with a leaf a type does
// not take. It is a reader of the Removal check (internal/domain/orchestration/removalcheck), the
// one package easement lets import it, so every removal that checks a row reads every holder. It
// imports its owner's context only; the segments module does not import it.
package segmentsuse

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
)

// UsingTag returns the Workspace's Segments whose Condition names the Tag by its name, in id order.
// Archived ones count: restoring one must not leave it naming a missing Tag. A Condition keeps the
// name, not an id, so deleting the Tag would leave it matching nothing.
func UsingTag(ctx context.Context, s *ent.Scoped, name string) ([]*ent.Segment, error) {
	all, err := s.Segment().QueryWithArchived().Order(ent.Asc(segment.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var using []*ent.Segment
	for _, seg := range all {
		g, err := condition.Parse(lo.FromPtr(seg.Condition))
		if err == nil && g.NamesTag(name) {
			using = append(using, seg)
		}
	}
	return using, nil
}

// UnfitForCustomFieldType returns the Workspace's Segments, in id order, whose Condition tests the
// Custom field key with a leaf a field of type t does not take, so they would stop compiling were
// the field retyped. Archived ones count, as for a Tag: restoring one must not bring back a
// Condition that no longer compiles.
func UnfitForCustomFieldType(ctx context.Context, s *ent.Scoped, key string, t customfield.Type) ([]*ent.Segment, error) {
	all, err := s.Segment().QueryWithArchived().Order(ent.Asc(segment.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Filter(all, func(seg *ent.Segment, _ int) bool {
		g, err := condition.Parse(lo.FromPtr(seg.Condition))
		return err == nil && !g.FitsCustomFieldType(key, t)
	}), nil
}
