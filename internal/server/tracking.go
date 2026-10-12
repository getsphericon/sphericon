package server

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/samber/lo"
)

// pixelGIF is a 1x1 transparent GIF served by the open-tracking endpoint.
var pixelGIF, _ = base64.StdEncoding.DecodeString(
	"R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7",
)

// trackingHandler serves the public email engagement endpoints:
//
//	GET  /e/o/{token}       — open pixel:       record open, return a 1x1 gif
//	GET  /e/c/{token}?u=URL — click:            record click, 302 to URL
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
// The token is a signed per-recipient JWT. Opens always return the pixel (even
// on a bad token) so we never leak token validity through the image.
func trackingHandler(client *ent.Client, bus *events.Bus, tracker *tracking.Tracker, limiter *ratelimit.Limiter) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /e/o/{token}", func(w http.ResponseWriter, r *http.Request) {
		if rid, err := tracker.Decode(r.PathValue("token")); err == nil && limiter.RecordsTracking(r) {
			recordOpen(r.Context(), client, bus, rid)
		}
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_, _ = w.Write(pixelGIF)
	})

	mux.HandleFunc("GET /e/c/{token}", func(w http.ResponseWriter, r *http.Request) {
		dest := r.URL.Query().Get("u")
		if !strings.HasPrefix(dest, "http://") && !strings.HasPrefix(dest, "https://") {
			http.Error(w, "invalid destination", http.StatusBadRequest)
			return
		}
		rid, err := tracker.Decode(r.PathValue("token"))
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
			return
		}
		if limiter.RecordsTracking(r) {
			recordClick(r.Context(), client, bus, rid, dest)
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
		// Safe method: record nothing, render the SPA confirmation page. The page's
		// button POSTs back to this same URL to perform the opt-out. For a per-source
		// opt-out we pass the deliberate "unsubscribe from everything" escalation link
		// so the page can offer it (a click, never an auto-fired request).
		http.Redirect(w, r, confirmRedirect(tracker, tokenStr, target), http.StatusSeeOther)
	})

	mux.HandleFunc("POST /e/u/{token}", func(w http.ResponseWriter, r *http.Request) {
		target, err := tracker.DecodeUnsub(r.PathValue("token"))
		if err != nil {
			http.Error(w, "invalid token", http.StatusBadRequest)
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
		if _, err := tracker.DecodeConfirm(tokenStr); err != nil {
			// Invalid or expired: send to the page with no token so it offers the
			// "request a new confirmation" path rather than a dead Confirm button.
			http.Redirect(w, r, "/confirm?expired=1", http.StatusSeeOther)
			return
		}
		// Safe method: record nothing, render the SPA confirmation page. The page's
		// button performs the confirmation through the site API.
		http.Redirect(w, r, "/confirm?token="+url.QueryEscape(tokenStr), http.StatusSeeOther)
	})

	return mux
}

// confirmRedirect is the SPA confirmation route the GET handler redirects to. It
// carries the token so the page's button can POST it back, and for a per-source
// opt-out the everything-scoped escalation URL (built from the same destination)
// so the page can offer it after confirming.
func confirmRedirect(tracker *tracking.Tracker, tokenStr string, target tracking.UnsubTarget) string {
	base := "/unsubscribe?token=" + url.QueryEscape(tokenStr)
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

// The record* helpers commit the state change and publish the engagement event in
// one transaction (transactional outbox). The state write is a conditional UPDATE
// (… WHERE opened_at IS NULL, etc.) and the counter increment + publish are gated
// on its rows-affected, so concurrent or redelivered hits are counted exactly once
// (no double-count race). The persist subscriber writes the Event log row and the
// automations subscriber enrolls the contact — both off the bus. Automation emails
// are sent untracked, so they emit no open/click events (no loop).

func recordOpen(ctx context.Context, client *ent.Client, bus *events.Bus, recipientID int64) {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if err != nil {
		return
	}
	c, err := client.Contact.Get(ctx, rec.ContactID)
	if err != nil {
		return
	}
	err = bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		n, err := tx.BroadcastRecipient.Update().
			Where(broadcastrecipient.ID(rec.ID), broadcastrecipient.OpenedAtIsNil()).
			SetOpenedAt(time.Now()).Save(ctx)
		if err != nil || n == 0 {
			return err // n == 0: already counted, nothing to publish
		}
		if _, err := tx.Broadcast.UpdateOneID(rec.BroadcastID).AddOpenedCount(1).Save(ctx); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.EmailEngagement{
			Action: events.NameEmailOpened, WorkspaceID: rec.WorkspaceID, ContactID: rec.ContactID,
			Email: lo.FromPtr(c.Email), BroadcastID: rec.BroadcastID,
		})
	})
	if err != nil {
		logging.FromContext(ctx).Error("tracking: record open failed", "recipient_id", recipientID, "err", err)
	}
}

func recordClick(ctx context.Context, client *ent.Client, bus *events.Bus, recipientID int64, dest string) {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if err != nil {
		return
	}
	c, err := client.Contact.Get(ctx, rec.ContactID)
	if err != nil {
		return
	}
	err = bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		n, err := tx.BroadcastRecipient.Update().
			Where(broadcastrecipient.ID(rec.ID), broadcastrecipient.ClickedAtIsNil()).
			SetClickedAt(time.Now()).Save(ctx)
		if err != nil || n == 0 {
			return err
		}
		if _, err := tx.Broadcast.UpdateOneID(rec.BroadcastID).AddClickedCount(1).Save(ctx); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.EmailEngagement{
			Action: events.NameEmailClicked, WorkspaceID: rec.WorkspaceID, ContactID: rec.ContactID,
			Email: lo.FromPtr(c.Email), BroadcastID: rec.BroadcastID, URL: dest,
		})
	})
	if err != nil {
		logging.FromContext(ctx).Error("tracking: record click failed", "recipient_id", recipientID, "err", err)
	}
}
