package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// SiteInstanceGetRelease is the newer release the Release check found, for the banner of a User
// who holds release:read in a live Workspace (ADR 0032); the module refuses everyone else. The
// Instance's settings are the Staff console's (ADR 0063).
func (h *Handlers) SiteInstanceGetRelease(ctx context.Context) (siteapi.SiteInstanceGetReleaseRes, error) {
	// The security handler lets no anonymous caller through; one would be no User, refused.
	var userID int64
	if a := auth.GetSiteAuth(ctx); a != nil {
		userID = a.UserID
	}
	release, err := h.instance.ReleaseFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return new(mapper.InstanceReleaseToResource(release)), nil
}
