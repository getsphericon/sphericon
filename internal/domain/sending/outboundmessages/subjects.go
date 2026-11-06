package outboundmessages

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// SubjectsOf is the subject each of the Events' emails kept, by Outbound message id: the one the
// recipient received, merge tags filled. An Event that names no message (events.OutboundMessageID),
// or whose message kept no subject (sent before it was kept, or erased with the Contact), has no
// entry.
func SubjectsOf(ctx context.Context, s *ent.Scoped, evs []*ent.Event) (map[int64]string, error) {
	ids := make([]int64, 0, len(evs))
	for _, e := range evs {
		if id, ok := events.OutboundMessageID(e); ok {
			ids = append(ids, id)
		}
	}
	subjects := map[int64]string{}
	if len(ids) == 0 {
		return subjects, nil
	}
	rows, err := s.OutboundMessage().Query().
		Where(outboundmessage.IDIn(ids...), outboundmessage.SubjectNotNil()).
		Select(outboundmessage.FieldID, outboundmessage.FieldSubject).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		subjects[r.ID] = *r.Subject
	}
	return subjects, nil
}
