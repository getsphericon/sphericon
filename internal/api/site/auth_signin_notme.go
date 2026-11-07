package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteAuthSigninNotMe is the link of the notice mailed after a silent link: it unlinks the
// identity the token names and ends every session of its User (ADR 0048). The token is the
// credential, so it works without a session; it works once, because once the identity is
// gone there is nothing left to undo.
func (h *Handlers) SiteAuthSigninNotMe(ctx context.Context, req *siteapi.SiteSigninNotMeInput) error {
	return h.links.UndoNotMe(ctx, req.Token)
}
