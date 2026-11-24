package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteEngagementTrackingGet reads the Engagement tracking consent of each Destination of the
// Contact, as the Contact card shows it (ADR 0120).
func (h *Handlers) SiteEngagementTrackingGet(ctx context.Context, params siteapi.SiteEngagementTrackingGetParams) (*siteapi.ContactEngagementTracking, error) {
	read, err := h.tracking.ReadContact(ctx, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactEngagementTrackingToResource(read)
	return &res, nil
}

// SiteEngagementTrackingCreate records a member's refusal or grant of tracking for one of the
// Contact's Destinations, as manual. A refusal applies from anyone; nothing here lifts one
// (ADR 0121).
func (h *Handlers) SiteEngagementTrackingCreate(ctx context.Context, req *siteapi.SiteRecordEngagementTrackingInput, params siteapi.SiteEngagementTrackingCreateParams) (*siteapi.EngagementTrackingObservationResult, error) {
	recorded, err := h.tracking.RecordByMember(ctx, workspaceScope(ctx), int64(params.ContactId), inputs.EngagementTrackingByMember(req))
	if err != nil {
		return nil, err
	}
	res := mapper.EngagementTrackingRecordedToResource(recorded)
	return &res, nil
}
