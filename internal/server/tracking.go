package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/machine"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// pixelGIF is a 1x1 transparent GIF served by the open-tracking endpoint.
var pixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x01, 0x44, 0x00, 0x3b,
}

// trackingHandler serves the public email engagement endpoints:
//
//	GET  /e/o/{token}       — open pixel:       record open, return a 1x1 gif
//	GET  /e/c/{token}       — click:            record click, 302 to the URL the token signs
//	GET  /e/u/{token}       — unsubscribe confirm: render the SPA page, record nothing
//	POST /e/u/{token}       — unsubscribe perform: opt the destination out of the scope
//	GET  /e/confirm/{token} — double opt-in confirm: render the SPA page, record nothing
//
// GET on the unsubscribe and confirm links is safe (ADR 0012 / RFC 8058, ADR 0013):
// it only redirects to the confirmation page, so link scanners and security proxies
// that GET every URL cannot unsubscribe or confirm anyone. The state change happens
// only on POST. Unsubscribe keeps POST /e/u/{token} here because it is the target of
// the mailbox one-click POST (RFC 8058); every other POST, from the pages' buttons,
// goes through the site API (POST /site/unsubscribes/{token},
// /site/confirmations/{token}), so the SPA stays on the generated client. Confirmation
// tokens additionally expire (~7 days).
//
// Tracking never refuses a recipient (ADR 0025): over the per-IP tracking guard an
// open still returns the pixel and a click still redirects, only the recording is
// skipped. Unsubscribe is not rate limited at all; its signed token protects it.
//
// The token is a signed JWT naming one Outbound message. Opens always return the pixel (even
// on a bad token) so we never leak token validity through the image.
func trackingHandler(client *ent.Client, bus *events.Bus, tracker *tracking.Tracker, limiter *ratelimit.Limiter, detector *machine.Detector, appURL string) http.Handler {
	mux := http.NewServeMux()
	appURL = strings.TrimRight(appURL, "/")

	mux.HandleFunc("GET /e/o/{token}", func(w http.ResponseWriter, r *http.Request) {
		mid, err := tracker.Decode(r.PathValue("token"))
		if err == nil && !messageOnHost(r, client, mid) {
			http.NotFound(w, r)
			return
		}
		if err == nil && limiter.RecordsTracking(r) {
			recordOpen(r.Context(), client, bus, mid, isMachine(r, detector))
		}
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_, _ = w.Write(pixelGIF)
	})

	mux.HandleFunc("GET /e/c/{token}", func(w http.ResponseWriter, r *http.Request) {
		mid, dest, err := tracker.DecodeClick(r.PathValue("token"))
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
		if !messageOnHost(r, client, mid) {
			http.NotFound(w, r)
			return
		}
		if limiter.RecordsTracking(r) {
			recordClick(r.Context(), client, bus, mid, dest, isMachine(r, detector))
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})

	mux.HandleFunc("GET /e/u/{token}", func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.PathValue("token")
		target, err := tracker.DecodeUnsub(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
		if !workspaceOnHost(r, target.WorkspaceID) {
			http.NotFound(w, r)
			return
		}
		// Safe method: record nothing, render the SPA confirmation page. The page's
		// button POSTs back to this same URL to perform the opt-out. For a per-source
		// opt-out we pass the deliberate "unsubscribe from everything" escalation link
		// so the page can offer it (a click, never an auto-fired request).
		http.Redirect(w, r, confirmRedirect(tracker, appURL, tokenStr, target), http.StatusSeeOther)
	})

	mux.HandleFunc("POST /e/u/{token}", func(w http.ResponseWriter, r *http.Request) {
		target, err := tracker.DecodeUnsub(r.PathValue("token"))
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
		if !workspaceOnHost(r, target.WorkspaceID) {
			http.NotFound(w, r)
			return
		}
		// Performs the opt-out — the target of the mailbox provider's one-click POST
		// (List-Unsubscribe=One-Click body) and the confirm page's button. No page is
		// returned; the SPA transitions its UI client-side on 204.
		if err := consent.RecordUnsubscribe(r.Context(), bus, target); err != nil {
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
		target, err := tracker.DecodeConfirm(tokenStr)
		if err == nil && !workspaceOnHost(r, target.WorkspaceID) {
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

// workspaceOnHost reports whether a request may act on workspaceID: always on the
// Instance's own host, and on a Tracking host only for that host's Workspace (ADR 0038).
func workspaceOnHost(r *http.Request, workspaceID int64) bool {
	h, ok := trackingHostFrom(r.Context())
	return !ok || h.workspaceID == workspaceID
}

// messageOnHost is workspaceOnHost for an open or click token, which carries no Workspace:
// it is read off the Outbound message row, and only when the request came in on a Tracking host.
func messageOnHost(r *http.Request, client *ent.Client, messageID int64) bool {
	h, ok := trackingHostFrom(r.Context())
	if !ok {
		return true
	}
	msg, err := client.OutboundMessage.Get(r.Context(), messageID)
	return err == nil && msg.WorkspaceID == h.workspaceID
}

// confirmRedirect is the absolute SPA confirmation URL (on the Instance's own host, so
// a Tracking host's visitor lands where the SPA is) the GET handler redirects to. It
// carries the token so the page's button can POST it back, and for a per-source
// opt-out the everything-scoped escalation URL (built from the same destination)
// so the page can offer it after confirming.
func confirmRedirect(tracker *tracking.Tracker, appURL, tokenStr string, target tracking.UnsubTarget) string {
	base := appURL + "/unsubscribe?token=" + url.QueryEscape(tokenStr)
	if target.Source == eligibility.SourceEverything {
		return base
	}
	allURL, err := tracker.UnsubscribeURL(tracking.UnsubTarget{
		Source:      eligibility.SourceEverything,
		Destination: target.Destination,
		WorkspaceID: target.WorkspaceID,
		ContactID:   target.ContactID,
	})
	if err != nil {
		return base
	}
	return base + "&all=" + url.QueryEscape(allURL)
}

// A machine hit (ADR 0053) is still an Event, flagged, but it sets no rollup stamp, so it reaches
// no figure.
//
// The record* helpers commit the rollup stamp and publish the engagement event in one
// transaction (transactional outbox). Every hit is an Event of its own, with its URL for a
// click (ADR 0053): a repeated open or a second click is a real engagement, not a duplicate.
// What is counted once is the Broadcast recipient's rollup, a conditional UPDATE
// (... WHERE opened_at IS NULL, in the broadcasts module) that backs the unique figures. The
// persist subscriber writes the Event log row and the automations subscriber enrolls the
// contact, both off the bus.

// engaged is the Outbound message a tracking token names, with the Contact it went to when
// there is one. A Transactional send to an address with no Contact is kept by address.
type engaged struct {
	msg     *ent.OutboundMessage
	contact *ent.Contact // nil: the message has no Contact
	email   string
	scope   *ent.Scoped
}

// loadEngaged resolves a token's message to its Workspace scope and Contact. A message whose
// Contact is gone (erased) records nothing, and neither does one whose address was cleared: those
// are expected and silent. A read that fails for any other reason records nothing too, and is
// logged, since the engagement is lost.
func loadEngaged(ctx context.Context, client *ent.Client, messageID int64) (engaged, bool) {
	msg, err := client.OutboundMessage.Get(ctx, messageID)
	if err != nil {
		logEngagedLookup(ctx, messageID, err)
		return engaged{}, false
	}
	s := client.Scoped(msg.WorkspaceID) // the Workspace comes from the message row the token names
	if msg.ContactID == nil {
		if msg.Destination == "" {
			return engaged{}, false
		}
		return engaged{msg: msg, email: msg.Destination, scope: s}, true
	}
	c, err := s.Contact().Get(ctx, *msg.ContactID)
	if err != nil {
		logEngagedLookup(ctx, messageID, err)
		return engaged{}, false
	}
	return engaged{msg: msg, contact: c, email: lo.FromPtr(c.Email), scope: s}, true
}

// logEngagedLookup logs a failed lookup of an engagement's message or Contact, unless the row is
// simply gone.
func logEngagedLookup(ctx context.Context, messageID int64, err error) {
	if !ent.IsNotFound(err) {
		logging.FromContext(ctx).Error("tracking: engagement lookup failed", "message_id", messageID, "err", err)
	}
}

func (e engaged) engagement(action, dest string, software bool) *events.EmailEngagement {
	var contactID int64 // 0: kept by address
	if e.contact != nil {
		contactID = e.contact.ID
	}
	return &events.EmailEngagement{
		Action: action, WorkspaceID: e.msg.WorkspaceID, ContactID: contactID,
		Email: e.email, BroadcastID: lo.FromPtr(e.msg.BroadcastID), URL: dest,
		AutomationID:      lo.FromPtr(e.msg.AutomationID),
		OutboundMessageID: e.msg.ID,
		Machine:           software,
	}
}

// isMachine judges a hit from its user-agent and client address (ADR 0053). Both are read for the
// verdict and dropped: neither reaches the Event or the log.
func isMachine(r *http.Request, detector *machine.Detector) bool {
	return detector.IsMachine(r.Context(), clientip.UserAgentFromContext(r.Context()), clientip.FromContext(r.Context()))
}

func recordOpen(ctx context.Context, client *ent.Client, bus *events.Bus, messageID int64, software bool) {
	e, ok := loadEngaged(ctx, client, messageID)
	if !ok {
		return
	}
	err := bus.WithinScopedTx(ctx, e.scope, func(ts *ent.Scoped, pub events.Publisher) error {
		if e.msg.BroadcastRecipientID != nil && !software {
			if _, err := broadcasts.RecordOpen(ctx, ts, *e.msg.BroadcastRecipientID); err != nil {
				return err
			}
		}
		return pub.Publish(ctx, e.engagement(events.NameEmailOpened, "", software))
	})
	if err != nil {
		logging.FromContext(ctx).Error("tracking: record open failed", "message_id", messageID, "err", err)
	}
}

func recordClick(ctx context.Context, client *ent.Client, bus *events.Bus, messageID int64, dest string, software bool) {
	e, ok := loadEngaged(ctx, client, messageID)
	if !ok {
		return
	}
	err := bus.WithinScopedTx(ctx, e.scope, func(ts *ent.Scoped, pub events.Publisher) error {
		if e.msg.BroadcastRecipientID != nil && !software {
			if _, err := broadcasts.RecordClick(ctx, ts, *e.msg.BroadcastRecipientID); err != nil {
				return err
			}
		}
		return pub.Publish(ctx, e.engagement(events.NameEmailClicked, dest, software))
	})
	if err != nil {
		logging.FromContext(ctx).Error("tracking: record click failed", "message_id", messageID, "err", err)
	}
}
