package callbackinterpretation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// ErrNoHook means no Integration of a live Workspace has the ingest key, or the one that has it
// reads no callbacks. A Workspace in its deletion window takes no new data (ADR 0043).
var ErrNoHook = errors.New("callbackinterpretation: no hook at this key")

// ErrUnconfirmed means the subscription the provider asked to confirm could not be confirmed.
// The provider asks again.
var ErrUnconfirmed = errors.New("callbackinterpretation: subscription confirmation failed")

// Callback is one request to the Ingest hook `POST /hooks/{key}` (ADR 0037).
type Callback struct {
	// Key is the ingest key of the hook URL: it belongs to one Integration and so names its
	// Workspace.
	Key string
	// Query is the raw query the request came with.
	Query  string
	Header http.Header
	Body   []byte
}

// Receive reads one callback. The Integration's Preset verifies the request and turns its payload
// into facts; a subscription the Preset asks to confirm is confirmed; the facts are interpreted in
// one transaction of the Integration's Workspace (Interpret), acting as the ingest actor.
//
// The refusals of the Preset come back as its own (preset.ErrUnauthentic, ErrUnavailable,
// ErrMalformed, ErrTransformFailed), beside ErrNoHook and ErrUnconfirmed; any other error is our
// own failure, and the provider retries. A valid request the Preset does not handle records
// nothing and is no error.
func (m *UseCase) Receive(ctx context.Context, in Callback) error {
	row, err := integrations.ByIngestKey(ctx, m.client, in.Key)
	if errors.Is(err, integrations.ErrNotFound) {
		return ErrNoHook
	}
	if err != nil {
		return fmt.Errorf("integration lookup: %w", err)
	}
	name := presetNameOf(row)
	if name == "" {
		// An Integration with no Preset reads no callbacks.
		return ErrNoHook
	}
	p, ok := m.integ.Presets().Get(name)
	if !ok {
		slog.ErrorContext(ctx, "ingest hook: the Integration's preset is not loaded", "integration_id", row.ID, "preset", name)
		return fmt.Errorf("the preset %q of integration %d is not loaded", name, row.ID)
	}
	params, err := m.integ.Params(row)
	if err != nil {
		slog.ErrorContext(ctx, "ingest hook: open the Integration's params failed", "integration_id", row.ID, "err", err)
		return fmt.Errorf("open the params of integration %d: %w", row.ID, err)
	}
	res, err := p.Handle(ctx, preset.Request{URL: m.requestURL(row, in.Query), Header: in.Header, Body: in.Body}, params, m.now())
	switch {
	case errors.Is(err, preset.ErrUnauthentic):
		// A forged callback (a STOP, a delivery report, a reply) is refused and left on record.
		slog.WarnContext(ctx, "ingest hook: refused a request that is not authentic", "integration_id", row.ID, "preset", p.Name())
		return err
	case errors.Is(err, preset.ErrUnavailable):
		// Our side failed (the signing certificate): the provider retries.
		slog.ErrorContext(ctx, "ingest hook: verification unavailable", "integration_id", row.ID, "preset", p.Name(), "err", err)
		return err
	case errors.Is(err, preset.ErrMalformed):
		return err
	case errors.Is(err, preset.ErrTransformFailed):
		// The request was verified, so the fault is the Preset's: the provider retries.
		slog.ErrorContext(ctx, "ingest hook: preset transform failed", "integration_id", row.ID, "preset", p.Name(), "err", err)
		return err
	case err != nil:
		slog.ErrorContext(ctx, "ingest hook: preset failed", "integration_id", row.ID, "preset", p.Name(), "err", err)
		return err
	}

	// The action is the engine's request to its caller: confirm a subscription at a URL.
	if res.Action != nil && res.Action.Kind == preset.ActionConfirm {
		if err := m.integ.ConfirmSubscription(ctx, res.Action.URL); err != nil {
			slog.ErrorContext(ctx, "ingest hook: confirm subscription failed", "integration_id", row.ID, "err", err)
			return fmt.Errorf("%w: %w", ErrUnconfirmed, err)
		}
	}

	// The Workspace comes from the ingest key, a secret of the Integration's (list item 6 of
	// docs/architecture/workspace-scoping.md, Workspace scoping).
	s := events.Ingest(m.client.Scoped(row.WorkspaceID))
	if err := m.Interpret(ctx, s, InterpretInput{Integration: row, Preset: p.Name(), Result: res}); err != nil {
		slog.ErrorContext(ctx, "ingest hook: interpreting the facts failed", "integration_id", row.ID, "err", err)
		return err
	}
	return nil
}

// presetNameOf is the Preset an Integration's callbacks are read with: the one it names, and for
// an SES Integration that names none, `ses` (it is implied, so an Integration made before Presets
// existed keeps receiving its bounces). "" when it reads none.
func presetNameOf(row *ent.Integration) string {
	switch {
	case row.Preset != nil:
		return *row.Preset
	case row.Provider == providers.SES:
		return preset.BuiltinSES
	}
	return ""
}

// requestURL is the full URL the provider posted to: the Integration's hook URL on the public
// address (what sphericon shows, and gives a provider to call), with the query the request came
// with. It is built from the configured address rather than the request's scheme and host, which
// a proxy in front of the server rewrites (behind Caddy the request arrives over plain HTTP).
func (m *UseCase) requestURL(row *ent.Integration, query string) string {
	u := m.integ.HookURL(row)
	if query != "" {
		u += "?" + query
	}
	return u
}
