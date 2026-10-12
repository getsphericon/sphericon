// Package ee assembles the Enterprise Edition (ADR 0014): everything under ee/ that
// the single binary plugs into core, gated by the runtime license key. The composition
// root (and the test harness) build one Edition and hand its parts to core seams, so
// there is exactly one place that decides what a license switches on.
//
// Governed by ee/LICENSE, not the AGPL.
package ee

import (
	"net/http"
	"time"

	entgo "entgo.io/ent"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/operator"
	"github.com/getsphericon/sphericon/ee/retention"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Edition is the Enterprise surface of one running instance.
type Edition struct {
	lic    *licensekey.License
	client *ent.Client

	// Audit is the read side of the Audit log; unlicensed, it reports so and the
	// /site page answers 402.
	Audit *audit.Log
	// Consumers are the extra bus subscribers (events.RegisterSubscribers).
	Consumers []events.Consumer
	// Operators is the platform Operator store and its login (ADR 0026); every call
	// refuses without the `operator` license.
	Operators *operator.Module

	operatorSessions *operator.Sessions
	now              func() time.Time
}

// New builds the Edition for a license. Subscribers are always registered and each
// checks the license itself, so a key added later needs no rewiring and an unlicensed
// instance stores nothing. The Operator configuration is validated only when the
// `operator` feature is licensed: an unlicensed instance needs none of it.
func New(client *ent.Client, lic *licensekey.License, cipher *secrets.Cipher, siteSecret string, operatorCfg operator.Config) (*Edition, error) {
	auditLog := audit.NewLog(lic)
	e := &Edition{
		lic:       lic,
		client:    client,
		Audit:     auditLog,
		Consumers: []events.Consumer{audit.Consumer(client, lic)},
		Operators: operator.NewModule(client, lic, auditLog, cipher, operatorCfg),
	}
	e.now = operatorCfg.Clock
	if e.now == nil {
		e.now = time.Now
	}
	if lic.Has(licensekey.FeatureOperator) {
		if err := operatorCfg.Validate(siteSecret); err != nil {
			return nil, err
		}
		e.operatorSessions = operator.NewSessions(client, lic, operatorCfg)
	}
	return e, nil
}

// Hooks are the ent hooks the Edition needs on every client, the long-lived one and the
// event bus's transaction client alike: the one that keeps the contacts peak of Usage as
// Contacts change (ADR 0009). Empty without the `operator` license.
func (e *Edition) Hooks() []entgo.Hook {
	if !e.lic.Has(licensekey.FeatureOperator) {
		return nil
	}
	return []entgo.Hook{usage.Hook(e.lic, e.now)}
}

// Operator is the /operator API (ADR 0026) for the composition root to mount, or nil
// without the `operator` license, in which case the whole surface answers 404.
//
// The surface suspends Workspaces through the core mechanism, so it takes the event bus
// and the system (platform) sender that tells the owner. The bus is built after the
// Edition (it carries the Edition's consumers), hence they come here and not to New.
func (e *Edition) Operator(bus *events.Bus, sender messaging.EmailSender) interface {
	Server(opts ...operatorapi.ServerOption) (http.Handler, error)
} {
	if e.operatorSessions == nil {
		return nil
	}
	return operator.NewSurface(e.Operators, e.operatorSessions, bus, sender)
}

// Service is the service endpoints of the Operator surface (ADR 0026) for the
// composition root to mount on the internal listener, or nil without the `operator`
// license, in which case no listener is bound and nothing is served.
func (e *Edition) Service(bus *events.Bus) interface {
	Server(opts ...serviceapi.ServerOption) (http.Handler, error)
} {
	if e.operatorSessions == nil {
		return nil
	}
	return operator.NewServiceSurface(e.Operators, bus)
}

// Freezer is the Operator holds' seat on the send chokepoint (ADR 0009, 0015): any
// active hold freezes a Workspace's sending, and a lapsed one is ignored at once. It
// honours holds only with the `operator` license, so an unlicensed instance never
// freezes on a row it could not have written.
func (e *Edition) Freezer() outbound.Freezer {
	return holds.Freezer{Now: e.now, Enabled: func() bool { return e.lic.Has(licensekey.FeatureOperator) }}
}

// Jobs is the Edition's river extension: the advanced-retention prune job (ADR 0014),
// which removes nothing without the retention license, and the usage job (ADR 0009),
// which materializes nothing without the operator license.
func (e *Edition) Jobs() jobs.Extension {
	return jobs.Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, retention.NewWorker(e.client, e.lic))
			river.AddWorker(w, usage.NewWorker(e.client, e.lic))
		},
		PeriodicJobs: []*river.PeriodicJob{retention.Periodic(), usage.Periodic()},
	}
}

// Webhooks gates the webhook dispatcher by license: `audit.entry` is forwarded to
// Webhook endpoints only when the Audit feature is licensed.
func (e *Edition) Webhooks(next events.WebhookDispatcher) events.WebhookDispatcher {
	return audit.Forwarder(next, e.lic)
}
