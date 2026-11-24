package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
)

// EngagementTrackingCreate records an Engagement tracking consent the customer collected, as
// imported (ADR 0120). A refusal applies from anyone; nothing here lifts one (ADR 0121).
func (h *Handlers) EngagementTrackingCreate(ctx context.Context, req *externalapi.RecordEngagementTrackingInput) (*externalapi.EngagementTrackingObservationResult, error) {
	in := inputs.EngagementTracking(req)
	in.Provenance = engagementtracking.Imported
	recorded, err := h.tracking.Record(ctx, auth.TokenScoped(ctx), in)
	if err != nil {
		return nil, err
	}
	res := mapper.EngagementTrackingRecordedToResource(recorded)
	return &res, nil
}

// EngagementTrackingGetForContact reads the Engagement tracking consent of each Destination of the
// Contact.
func (h *Handlers) EngagementTrackingGetForContact(ctx context.Context, params externalapi.EngagementTrackingGetForContactParams) (*externalapi.ContactEngagementTracking, error) {
	read, err := h.tracking.ReadContact(ctx, auth.TokenScoped(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactEngagementTrackingToResource(read)
	return &res, nil
}
