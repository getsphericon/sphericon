package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/preset"
)

// hooksHandler serves inbound provider callbacks (ADR 0037): POST /hooks/{key}. The key
// belongs to one Integration and so names its Workspace. This is a composition root of
// the scoped client (see AGENTS.md): it resolves the Integration by key with the raw
// client, then scopes to the Integration's Workspace.
//
// A Workspace in its deletion window takes no new data (ADR 0043): its keys answer 404.
//
// The Integration's Preset verifies the request (a bad signature or wrong source is 4xx)
// and turns the payload into Delivery failures, which are published through the bus in
// one transaction before the 2xx; our own failure is 5xx so the provider retries; a valid
// request the Preset does not handle is 2xx with nothing published.
func hooksHandler(client *ent.Client, bus *events.Bus, integ *integrations.Module, now func() time.Time) http.Handler {
	if now == nil {
		now = time.Now
	}
	h := &hook{
		ent:   client,
		bus:   bus,
		integ: integ,
		now:   now,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{key}", h.handle)
	return mux
}

type hook struct {
	ent   *ent.Client
	bus   *events.Bus
	integ *integrations.Module
	now   func() time.Time
}

func (h *hook) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	row, err := h.ent.Integration.Query().
		Where(integration.IngestKey(r.PathValue("key")), integration.HasWorkspaceWith(accounts.Live())).
		Only(ctx)
	if ent.IsNotFound(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "integration lookup failed")
		return
	}

	// Bounded to what the engine parses, so an oversized body is never held in memory.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, preset.MaxBodyBytes))
	if err != nil {
		if tooBig, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeProblem(w, http.StatusRequestEntityTooLarge, tooBig.Error())
			return
		}
		writeProblem(w, http.StatusBadRequest, "read body")
		return
	}

	name := presetNameOf(row)
	if name == "" {
		// An Integration with no Preset reads no callbacks.
		http.NotFound(w, r)
		return
	}
	h.servePreset(w, r, row, name, body)
}

// presetNameOf is the Preset an Integration's callbacks are read with: the one it names,
// and for an SES Integration that names none, `ses` (it is implied, so an Integration made
// before Presets existed keeps receiving its bounces). "" when it reads none.
func presetNameOf(row *ent.Integration) string {
	switch {
	case row.Preset != nil:
		return *row.Preset
	case row.Provider == integration.ProviderSes:
		return "ses"
	}
	return ""
}

func (h *hook) servePreset(w http.ResponseWriter, r *http.Request, row *ent.Integration, name string, body []byte) {
	ctx := r.Context()
	log := logging.FromContext(ctx)
	p, ok := h.integ.Presets().Get(name)
	if !ok {
		log.Error("hooks: the Integration's preset is not loaded", "integration_id", row.ID, "preset", name)
		writeProblem(w, http.StatusInternalServerError, "the preset of this integration is not available")
		return
	}
	params, err := h.integ.Params(row)
	if err != nil {
		log.Error("hooks: open the Integration's params failed", "integration_id", row.ID, "err", err)
		writeProblem(w, http.StatusInternalServerError, "integration params unavailable")
		return
	}
	res, err := p.Handle(ctx, preset.Request{Header: r.Header, Body: body}, params, h.now())
	switch {
	case errors.Is(err, preset.ErrUnauthentic):
		writeProblem(w, http.StatusForbidden, "the request is not authentic")
		return
	case errors.Is(err, preset.ErrUnavailable):
		// Our side failed (the signing certificate): the provider retries.
		log.Error("hooks: verification unavailable", "integration_id", row.ID, "preset", p.Name(), "err", err)
		writeProblem(w, http.StatusBadGateway, "the request could not be verified right now")
		return
	case errors.Is(err, preset.ErrMalformed):
		writeProblem(w, http.StatusBadRequest, "the request is malformed")
		return
	case errors.Is(err, preset.ErrTransformFailed):
		// The request was verified, so the fault is the Preset's: the provider retries.
		log.Error("hooks: preset transform failed", "integration_id", row.ID, "preset", p.Name(), "err", err)
		writeProblem(w, http.StatusInternalServerError, "the request could not be read")
		return
	case err != nil:
		log.Error("hooks: preset failed", "integration_id", row.ID, "preset", p.Name(), "err", err)
		writeProblem(w, http.StatusInternalServerError, "preset failed")
		return
	}

	// The action is the engine's request to its caller: confirm a subscription at a URL.
	if res.Action != nil && res.Action.Kind == preset.ActionConfirm {
		if err := h.integ.ConfirmSubscription(ctx, res.Action.URL); err != nil {
			log.Error("hooks: confirm subscription failed", "integration_id", row.ID, "err", err)
			writeProblem(w, http.StatusBadGateway, "subscription confirmation failed")
			return
		}
	}

	s := events.Ingest(h.ent.Scoped(row.WorkspaceID))
	if err := h.publish(ctx, s, p.Name(), res.Failures); err != nil {
		log.Error("hooks: publishing delivery failures failed", "integration_id", row.ID, "err", err)
		writeProblem(w, http.StatusInternalServerError, "processing failed")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// publish writes the Delivery failures to the outbox in one transaction: a redelivery of
// the same provider event dedupes downstream on its DedupID (the provider's event id plus
// the Destination). A transient bounce is published too (the deliverability rates read
// it) and never enters Suppression.
func (h *hook) publish(ctx context.Context, s *ent.Scoped, provider string, failures []preset.DeliveryFailure) error {
	if len(failures) == 0 {
		return nil
	}
	evs := make([]*events.EmailDeliveryFailure, 0, len(failures))
	for _, f := range failures {
		email := strings.ToLower(strings.TrimSpace(f.Destination))
		if email == "" {
			continue
		}
		var contactID int64
		c, err := s.Contact().Query().Where(contact.EmailEqualFold(email)).Only(ctx)
		switch {
		case err == nil:
			contactID = c.ID
		case ent.IsNotFound(err):
			// An address that is no Contact is suppressed anyway, with no Contact link.
		default:
			return fmt.Errorf("lookup contact %q: %w", email, err)
		}
		ev := &events.EmailDeliveryFailure{
			Action:        events.NameEmailBounced,
			WorkspaceID:   s.WorkspaceID(),
			ContactID:     contactID,
			Email:         email,
			Provider:      provider,
			SendingDomain: f.SendingDomain,
			DedupID:       f.ID + "/" + email,
		}
		if f.Kind == preset.Complaint {
			ev.Action = events.NameEmailComplained
		} else {
			ev.BounceKind = events.BounceKindTransient
			if f.Permanent {
				ev.BounceKind = events.BounceKindPermanent
			}
		}
		evs = append(evs, ev)
	}
	return h.bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		for _, ev := range evs {
			if err := pub.Publish(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}
