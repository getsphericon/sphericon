package outbound

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrRouteHeld refuses to arm a send (an Automation's Publish, a Broadcast's Schedule) whose Send
// route cannot send: the Integration it names is disabled or archived, or the Workspace has no
// default to send through. Drafts and edits may still name such an Integration; only the moment a
// send is armed is refused, and a route that stops sending after it holds the send instead.
var ErrRouteHeld = validation.Conflict("outbound: the send route cannot send", "errors.send_route_held")

// Route is the Send route of an Integration (GLOSSARY): the one answer to which Integration a send
// goes through, with what sender, at what pace, and whether it can send at all. Every send asks it
// (Send, a Test send, the Broadcast planner's Preflight and pace, Publish and Schedule), so the rule
// "a named Integration that cannot send holds the send, never the default in its place" is written
// once, in Route.
type Route struct {
	// Integration is the Integration the send goes through: the one named, else the Workspace's
	// default. Nil when the route is held, or when the sender has no Integration row (a test double).
	Integration *ent.Integration
	// Sender is the provider adapter built for the Integration; nil when the route is held.
	Sender messaging.EmailSender
	// DeliversOutside is the sender's own word that its mail leaves sphericon, so a send through it
	// needs a verified Sending domain (ADR 0058). A sender that keeps its mail (a Test inbox) does not.
	DeliversOutside bool
	// Limits are the Integration's effective Provider limit (ADR 0023): none without an Integration
	// row.
	Limits providerlimit.Limits
	// Hold is why nothing can go through the route (HoldNoIntegration, HoldProviderStopped); ""
	// when it can send.
	Hold string
}

// Held reports whether nothing can go through the route.
func (r Route) Held() bool { return r.Hold != "" }

// Interval is the step between two sends through the route at its Provider limit: zero when nothing
// limits it. The Broadcast planner paces by it, and the progress estimate reads it.
func (r Route) Interval() time.Duration { return r.Limits.Interval() }

// Armable refuses to arm a send through a route with no Integration that can send, with
// ErrRouteHeld. A provider that has stopped the account (HoldProviderStopped) does not refuse it:
// that standing lifts by itself once a quota read finds the account sending again, and the sends
// wait for it.
func (r Route) Armable() error {
	if r.Hold == HoldNoIntegration {
		return ErrRouteHeld
	}
	return nil
}

// Route resolves the Send route of the Integration integrationID names, else of the Workspace's
// default. A named Integration that is disabled, archived or of another Workspace holds the route
// (HoldNoIntegration) and never falls back to the default; so does a Workspace with no default, or
// an Integration the provider adapter cannot be built for (messaging.ErrNoProvider). An Integration
// whose provider has stopped its account holds it too (HoldProviderStopped). It says nothing
// of the Workspace freeze or the From's Sending domain: those are the send's own checks (gate).
func (m *Module) Route(ctx context.Context, s *ent.Scoped, integrationID *int64) (Route, error) {
	integ, err := messaging.EmailIntegration(ctx, s, integrationID)
	if err != nil && !errors.Is(err, messaging.ErrNoProvider) {
		return Route{}, fmt.Errorf("outbound: load integration: %w", err)
	}
	if integ == nil && integrationID != nil {
		return Route{Hold: HoldNoIntegration}, nil
	}
	if integ != nil && providerlimit.Stopped(integ) {
		return Route{Integration: integ, Limits: providerlimit.EffectiveOf(integ).Limits(), Hold: HoldProviderStopped}, nil
	}
	// Without a default row the sender is still asked: a test double sends for any Workspace.
	sender, err := m.senders.EmailSender(ctx, s, integ)
	if errors.Is(err, messaging.ErrNoProvider) {
		return Route{Hold: HoldNoIntegration}, nil
	}
	if err != nil {
		return Route{}, fmt.Errorf("outbound: resolve sender: %w", err)
	}
	r := Route{Integration: integ, Sender: sender, DeliversOutside: messaging.DeliversOutside(sender)}
	if integ != nil {
		r.Limits = providerlimit.EffectiveOf(integ).Limits()
	}
	return r, nil
}
