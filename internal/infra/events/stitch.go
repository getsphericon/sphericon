package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
)

// StitchVisitor attaches the Visitor visitorID's earlier anonymous Events to the Contact contactID
// (GLOSSARY: Identify), so pre-identify behavior becomes visible to segmentation. An Event that
// already names a Contact keeps it.
func StitchVisitor(ctx context.Context, s *ent.Scoped, visitorID string, contactID int64) error {
	_, err := s.Event().Update().
		Where(
			event.VisitorID(visitorID),
			event.ContactIDIsNil(),
		).
		SetContactID(contactID).
		Save(ctx)
	return err
}
