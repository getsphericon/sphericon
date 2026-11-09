// Package tracking serves the public links of the messages sphericon sends, a driver beside the
// API surfaces (ADR 0111):
//
//	GET  /e/o/{token}       — open pixel:       record open, return a 1x1 gif
//	GET  /e/c/{token}       — click:            record click, 302 to the URL the token signs
//	GET  /e/u/{token}       — unsubscribe confirm: render the SPA page, record nothing
//	POST /e/u/{token}       — unsubscribe perform: opt the destination out of the scope
//	GET  /e/confirm/{token} — double opt-in confirm: render the SPA page, record nothing
//	GET  /c/{token}         — short tracked link of a Telegram message or an SMS (ADR 0083): record
//	                          the click, 302 to the URL stored with the token; unknown token: 404;
//	                          a Test send's link (no message): 302, nothing recorded
//
// GET on the unsubscribe and confirm links is safe (ADR 0012 / RFC 8058, ADR 0013): it only
// redirects to the confirmation page, so link scanners and security proxies that GET every URL
// cannot unsubscribe or confirm anyone. The state change happens only on POST. Unsubscribe keeps
// POST /e/u/{token} here because it is the target of the mailbox one-click POST (RFC 8058); every
// other POST, from the pages' buttons, goes through the site API (POST /site/unsubscribes/{token},
// /site/confirmations/{token}), so the SPA stays on the generated client. Confirmation tokens
// additionally expire (~7 days).
//
// Tracking never refuses a recipient (ADR 0025): over the per-IP tracking guard an open still
// returns the pixel and a click still redirects, only the recording is skipped. Unsubscribe is not
// rate limited at all; its signed token protects it.
//
// An open or a click is recorded by the Outbound message's engagement (ADR 0053): opens always
// return the pixel, even on a bad token, so the image never leaks whether a token is valid. On a
// Tracking host (ADR 0038) a link of another Workspace is not found.
package tracking

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/clientip"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// pixelGIF is a 1x1 transparent GIF served by the open-tracking endpoint.
var pixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x01, 0x44, 0x00, 0x3b,
}

// Deps is what the links are served with.
type Deps struct {
	Engagement *engagement.Recorder
	// Tracker decodes the unsubscribe and confirmation tokens and builds their links.
	Tracker     *messagetoken.Tracker
	Unsubscribe *unsubscriberecording.UseCase
	// Workspaces builds the scope a signed unsubscribe link names.
	Workspaces *workspaces.Module
	// Limiter is the per-IP tracking guard (ADR 0025).
	Limiter *ratelimit.Limiter
	// AppURL is the Instance's own address, where the SPA's confirmation pages are.
	AppURL string
}

// New is the handler of the links (`/e/` and `/c/`).
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	appURL := strings.TrimRight(d.AppURL, "/")

	mux.HandleFunc("GET /e/o/{token}", func(w http.ResponseWriter, r *http.Request) {
		err := d.Engagement.Open(r.Context(), r.PathValue("token"), hit(r, d.Limiter))
		switch {
		case errors.Is(err, engagement.ErrNotFound):
			http.NotFound(w, r)
			return
		case err != nil && !errors.Is(err, engagement.ErrInvalidToken):
			// The pixel is answered all the same: an open the Recorder could not check records
			// nothing, and the image shows nothing of the message.
			logging.FromContext(r.Context()).Error("tracking: open lookup failed", "err", err)
		}
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_, _ = w.Write(pixelGIF)
	})

	mux.HandleFunc("GET /e/c/{token}", func(w http.ResponseWriter, r *http.Request) {
		dest, err := d.Engagement.Click(r.Context(), r.PathValue("token"), hit(r, d.Limiter))
		switch {
		case errors.Is(err, engagement.ErrInvalidToken):
			http.Error(w, "invalid token", http.StatusBadRequest)
		case errors.Is(err, engagement.ErrNotFound):
			http.NotFound(w, r)
		case err != nil:
			logging.FromContext(r.Context()).Error("tracking: click lookup failed", "err", err)
			http.Error(w, "lookup failed", http.StatusInternalServerError)
		default:
			http.Redirect(w, r, dest, http.StatusFound)
		}
	})

	mux.HandleFunc("GET /c/{token}", func(w http.ResponseWriter, r *http.Request) {
		dest, err := d.Engagement.ShortLink(r.Context(), r.PathValue("token"), hit(r, d.Limiter))
		switch {
		case errors.Is(err, engagement.ErrNotFound):
			http.NotFound(w, r)
		case err != nil:
			logging.FromContext(r.Context()).Error("tracking: short link lookup failed", "err", err)
			http.Error(w, "lookup failed", http.StatusInternalServerError)
		default:
			http.Redirect(w, r, dest, http.StatusFound)
		}
	})

	mux.HandleFunc("GET /e/u/{token}", func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.PathValue("token")
		target, err := d.Tracker.DecodeUnsub(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
		if !onHost(r, target.WorkspaceID) {
			http.NotFound(w, r)
			return
		}
		// Safe method: record nothing, render the SPA confirmation page. The page's
		// button POSTs back to this same URL to perform the opt-out. For a per-source
		// opt-out we pass the deliberate "unsubscribe from everything" escalation link
		// so the page can offer it (a click, never an auto-fired request).
		http.Redirect(w, r, confirmRedirect(d.Tracker, appURL, tokenStr, target), http.StatusSeeOther)
	})

	mux.HandleFunc("POST /e/u/{token}", func(w http.ResponseWriter, r *http.Request) {
		target, err := d.Tracker.DecodeUnsub(r.PathValue("token"))
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
		if !onHost(r, target.WorkspaceID) {
			http.NotFound(w, r)
			return
		}
		// Performs the opt-out — the target of the mailbox provider's one-click POST
		// (List-Unsubscribe=One-Click body) and the confirm page's button. No page is
		// returned; the SPA transitions its UI client-side on 204.
		if err := d.Unsubscribe.RecordLink(r.Context(), d.Workspaces.LinkScope(target.WorkspaceID), target); err != nil {
			logging.FromContext(r.Context()).Error("tracking: unsubscribe failed", "workspace_id", target.WorkspaceID, "contact_id", target.ContactID, "source", target.Source, "err", err)
			// Not a 204: a mailbox provider must see the failure and retry, and a
			// person must not be told they are unsubscribed when nothing was recorded.
			http.Error(w, "unsubscribe failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /e/confirm/{token}", func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.PathValue("token")
		target, err := d.Tracker.DecodeConfirm(tokenStr)
		if err == nil && !onHost(r, target.WorkspaceID) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			// Invalid or expired: send to the page with no token so it offers the
			// "request a new confirmation" path rather than a dead Confirm button.
			http.Redirect(w, r, appURL+"/confirm?expired=1", http.StatusSeeOther)
			return
		}
		// Safe method: record nothing, render the SPA confirmation page. The page's
		// button performs the confirmation through the site API.
		http.Redirect(w, r, appURL+"/confirm?token="+url.QueryEscape(tokenStr), http.StatusSeeOther)
	})

	return mux
}

// hit is the request as the engagement reads it: its user-agent and client address, the Tracking
// host it came in on, and the per-IP tracking guard that says whether it may be recorded
// (ADR 0025).
func hit(r *http.Request, limiter *ratelimit.Limiter) engagement.Hit {
	ctx := r.Context()
	return engagement.Hit{
		UserAgent: clientip.UserAgentFromContext(ctx),
		Address:   clientip.FromContext(ctx),
		Host:      auth.HostWorkspace(ctx),
		MayRecord: func() bool { return limiter.RecordsTracking(r) },
	}
}

// onHost reports whether a request may act on workspaceID: always on the Instance's own host, and
// on a Tracking host only for that host's Workspace (ADR 0038).
func onHost(r *http.Request, workspaceID int64) bool {
	host := auth.HostWorkspace(r.Context())
	return host == 0 || host == workspaceID
}

// confirmRedirect is the absolute SPA confirmation URL (on the Instance's own host, so a Tracking
// host's visitor lands where the SPA is) the GET handler redirects to. It carries the token so the
// page's button can POST it back, the Channel the link came on so the page speaks of emails, texts
// or Telegram messages, and for a per-source opt-out the everything-scoped escalation URL (built
// from the same destination) so the page can offer it after confirming.
func confirmRedirect(tracker *messagetoken.Tracker, appURL, tokenStr string, target linktarget.Unsubscribe) string {
	base := appURL + "/unsubscribe?token=" + url.QueryEscape(tokenStr) + "&channel=" + url.QueryEscape(string(target.Channel))
	if target.Source == eligibility.SourceEverything {
		return base
	}
	allURL, err := tracker.UnsubscribeURL(linktarget.Unsubscribe{
		Channel:     target.Channel,
		Source:      eligibility.SourceEverything,
		Destination: target.Destination,
		WorkspaceID: target.WorkspaceID,
		ContactID:   target.ContactID,
		// The same email's link: opting out of everything from it is about that message too.
		OutboundMessageID: target.OutboundMessageID,
	})
	if err != nil {
		return base
	}
	return base + "&all=" + url.QueryEscape(allURL)
}
