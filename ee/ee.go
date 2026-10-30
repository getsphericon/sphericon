// Package ee assembles the Enterprise Edition (ADR 0014): everything under ee/ that
// the single binary plugs into core, gated by the runtime license key. The composition
// root (and the test harness) build one Edition and hand its parts to core seams, so
// there is exactly one place that decides what a license switches on.
//
// Governed by ee/LICENSE, not the AGPL.
package ee

import (
	"context"
	"errors"
	"net/http"
	"time"

	entgo "entgo.io/ent"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/export"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/retention"
	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/abusewatch"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/staffmembers"
)

// Edition is the Enterprise surface of one running instance.
type Edition struct {
	lic    *licensekey.License
	client *ent.Client

	// Audit is the read side of the Audit log; unlicensed, it reports so and the
	// /site page answers 402.
	Audit *audit.Log
	// Export is the seat of the Export streams (ADR 0045); unlicensed, every stream answers 404.
	Export *export.Streams
	// Consumers are the extra bus subscribers (events.RegisterSubscribers).
	Consumers []events.Consumer
	// Staff is the domain of the console's Enterprise sections and of the service endpoints
	// (ADR 0026); every call refuses without the `staff` license.
	Staff *staff.Module

	staffSessions *staffmembers.Sessions
	abuseCfg      abuse.Config
	now           func() time.Time
}

// New builds the Edition for a license. Subscribers are always registered and each
// checks the license itself, so a key added later needs no rewiring and an unlicensed
// instance stores nothing. The Enterprise sections of the Staff console take the core Staff
// member's sessions (staffSessions, nil while its sign-in is off), so the `staff` feature
// refuses to start without them. The Audit log records its export through bus, which is
// built before the Edition and takes the Edition's ent hooks from Hooks.
func New(client *ent.Client, bus *events.Bus, lic *licensekey.License, staffSessions *staffmembers.Sessions, staffCfg staff.Config) (*Edition, error) {
	auditLog := audit.NewLog(lic, bus)
	e := &Edition{
		lic:       lic,
		client:    client,
		Audit:     auditLog,
		Export:    export.New(lic),
		Consumers: []events.Consumer{audit.Consumer(client, lic), export.Consumer(client, lic)},
		Staff:     staff.NewModule(client, lic, auditLog, staffCfg),
	}
	e.abuseCfg = abuse.Config{Limits: staffCfg.Abuse, SupportEmail: staffCfg.SupportEmail}
	e.now = staffCfg.Clock
	if e.now == nil {
		e.now = time.Now
	}
	if lic.Has(licensekey.FeatureStaff) {
		if staffSessions == nil {
			return nil, errors.New("SPHERICON_STAFF_JWT_SECRET is required with the staff license: the console's Enterprise sections need the Staff member's sign-in")
		}
		e.staffSessions = staffSessions
	}
	return e, nil
}

// Hooks are the ent hooks the Edition needs on every client, the long-lived one and the
// event bus's transaction client alike: the one that keeps the contacts peak of Usage as
// Contacts change (ADR 0009). Empty without the `staff` license.
func (e *Edition) Hooks() []entgo.Hook { return Hooks(e.lic, e.now) }

// Hooks are the Edition's ent hooks for a license and a clock, for a client built before
// the Edition (the event bus's transaction client). Empty without the `staff` license.
func Hooks(lic *licensekey.License, now func() time.Time) []entgo.Hook {
	if !lic.Has(licensekey.FeatureStaff) {
		return nil
	}
	return []entgo.Hook{usage.Hook(lic, now)}
}

// StaffSections is the Enterprise part of the /staff API (ADR 0026, ADR 0063) for the
// composition root to mount behind the core one, or nil without the `staff` license, in
// which case its routes answer 404.
//
// The surface suspends Workspaces through the core mechanism, so it takes the event bus
// and the system (platform) sender that tells the owner.
func (e *Edition) StaffSections(bus *events.Bus, sender messaging.EmailSender) interface {
	Server(opts ...staffeeapi.ServerOption) (http.Handler, error)
} {
	if e.staffSessions == nil {
		return nil
	}
	return staff.NewSurface(e.Staff, e.staffSessions, bus, sender)
}

// Service is the service endpoints of the Staff surface (ADR 0026) for the
// composition root to mount on the internal listener, or nil without the `staff`
// license, in which case no listener is bound and nothing is served.
func (e *Edition) Service(bus *events.Bus) interface {
	Server(opts ...serviceapi.ServerOption) (http.Handler, error)
} {
	if e.staffSessions == nil {
		return nil
	}
	return staff.NewServiceSurface(e.Staff, bus)
}

// Holds is the read side of a Workspace's freezes on the send chokepoint (ADR 0009, 0015).
// It honours Staff holds only with the `staff` license, so an unlicensed instance never
// freezes on a row it could not have written; deletion and suspension always count.
func (e *Edition) Holds() *holds.Holds { return holds.New(e.HoldsHonoured, e.now) }

// HoldsHonoured reports whether Staff holds are honoured at all (the `staff`
// license): the send path and every read of a hold shown to a Workspace
// agree on it.
func (e *Edition) HoldsHonoured() bool { return e.lic.Has(licensekey.FeatureStaff) }

// StaffLicensed reports whether the `staff` license is held: what the Sign-up survey needs,
// since only the Staff console reads its answers.
func (e *Edition) StaffLicensed() bool { return e.lic.Has(licensekey.FeatureStaff) }

// LicenseLapsedAt is when the license key expired, false unless it has lapsed (ADR 0035).
func (e *Edition) LicenseLapsedAt() (time.Time, bool) { return e.lic.LapsedAt() }

// Abuse is the abuse detector (ADR 0026): the pass the hourly job runs, and the read of
// a Workspace's standing. It suspends through the core mechanism, so it takes the event
// bus, and tells owners through the system (platform) sender. Unlicensed it does nothing.
func (e *Edition) Abuse(bus *events.Bus, sender messaging.EmailSender) *abuse.Detector {
	return abuse.New(e.client, e.lic, bus, sender, e.abuseCfg)
}

// AbuseWatch is the read side of the detector for /site: the Workspace's warning, none
// without the `staff` license.
func (e *Edition) AbuseWatch() abusewatch.Watch { return abuseWatchAdapter{e.Staff.Watch()} }

type abuseWatchAdapter struct{ watch *abuse.Watch }

func (w abuseWatchAdapter) Warning(ctx context.Context, s *ent.Scoped, now time.Time) (*abusewatch.Warning, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	b, err := w.watch.Warning(ctx, s, ws, now)
	if err != nil || b.Tier == abuse.TierNone {
		return nil, err
	}
	return &abusewatch.Warning{Domain: b.Domain, Metric: string(b.Metric), Rate: b.Rate()}, nil
}

// Jobs is the Edition's river extension: the advanced-retention prune job (ADR 0014),
// which removes nothing without the retention license, the usage job (ADR 0009) and the
// abuse detector (ADR 0026), which do nothing without the staff license.
func (e *Edition) Jobs(bus *events.Bus, sender messaging.EmailSender) jobs.Extension {
	detector := e.Abuse(bus, sender)
	return jobs.Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, retention.NewWorker(e.client, e.lic))
			river.AddWorker(w, usage.NewWorker(e.client, e.lic))
			river.AddWorker(w, abuse.NewWorker(detector))
		},
		PeriodicJobs: []*river.PeriodicJob{retention.Periodic(), usage.Periodic(), abuse.Periodic()},
	}
}

// ExportJobs is the river extension of the Export streams (ADR 0045): the prune job of the
// Erasures stream over the instance's Event retention window (0 keeps every row). It is
// separate from Jobs because it needs that window, a core setting; it removes nothing
// without the `export` license.
func (e *Edition) ExportJobs(eventsRetention time.Duration) jobs.Extension {
	return export.Jobs(e.client, e.lic, eventsRetention)
}

// Webhooks gates the webhook dispatcher by license: `audit.entry` is forwarded to
// Webhook endpoints only when the Audit feature is licensed.
func (e *Edition) Webhooks(next events.WebhookDispatcher) events.WebhookDispatcher {
	return audit.Forwarder(next, e.lic)
}
