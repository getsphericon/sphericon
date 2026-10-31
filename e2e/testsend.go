//go:build e2e

package e2e

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// cookieSource is the signed-in session of the SPA's API.
type cookieSource string

func (c cookieSource) ApiKeyAuth(context.Context, siteapi.OperationName) (siteapi.ApiKeyAuth, error) {
	return siteapi.ApiKeyAuth{APIKey: string(c)}, nil
}

// RequestTestSend creates a draft Broadcast and, as the Workspace's owner, asks for a Test
// send of it to the owner's own address, the way the editor does. It returns the From address
// the answer says the preview went out from.
func (w *Workspace) RequestTestSend(b Broadcast) string {
	w.t.Helper()
	created := w.createBroadcast(b)
	res, err := w.site.SiteBroadcastsTestSend(w.t.Context(),
		&siteapi.TestSendBroadcastInput{Email: siteapi.EmailAddress(w.OwnerEmail)},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: siteapi.WorkspaceId(w.publicID), ID: siteapi.EntityId(created)})
	sent := ok[siteapi.SiteTestSendBroadcastResult](w.t, "test send", res, err)
	return string(sent.From)
}
