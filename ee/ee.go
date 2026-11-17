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

	"github.com/getsphericon/sphericon/ee/domain/insights/erasures"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"

	entgo "entgo.io/ent"

	staffhandlers "github.com/getsphericon/sphericon/ee/api/staff"
	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/insights/abusewarning"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/reputation"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// ExportStreams is the Enterprise side of the Export streams (ADR 0045): the `export` license
// gate core asks before it serves a stream. The paging, the cursor and the row encoding are core
// (internal/domain/insights/exportstream); the stream that needs Enterprise state, the Erasures,
// is ee/domain/insights/erasures.
type ExportStreams struct {
	lic *licensekey.License
}

// Licensed reports whether the `export` feature is licensed; unlicensed, every stream
// answers 404 like the /staff surface (ADR 0045).
func (s *ExportStreams) Licensed() bool { return s != nil && s.lic.Has(licensekey.FeatureExport) }

// Edition is the Enterprise surface of one running instance.
type Edition struct {
	lic    *licensekey.License
	client *ent.Client

	// Audit is the Audit log: its consumer, which the Enterprise jobs run, and its read side
	// (auditlog.Reader), which auditlog.Log serves the surfaces over; unlicensed, it reports so and
	// the Log refuses as unlicensed (402).
	Audit *auditentries.Log
	// Erasures is the Erasures stream's store (ADR 0045), which the Enterprise jobs fill and prune;
	// unlicensed, it stores and removes nothing.
	Erasures *erasures.Module
	// Usage is the Usage of every Workspace (ADR 0009), which the Enterprise jobs refresh and the
	// Staff module reads; unlicensed, the refresh does nothing.
	Usage *usages.Module
	// Export is the seat of the Export streams (ADR 0045); unlicensed, every stream answers 404.
	Export *ExportStreams
	// Staff is the module of the console's Enterprise sections and of the service endpoints
	// (ADR 0026); every call refuses without the `staff` license.
	Staff *workspaceoversight.Module
	// ServiceCredentials issues and checks the Service credentials of the service endpoints
	// (ADR 0026); every call refuses without the `staff` license.
	ServiceCredentials *servicecredentials.Module

	staffSessions *staffmembers.Sessions
	bus           *events.Bus
	suspend       *suspension.Suspender
	workspaces    *workspaces.Module
	holds         *holds.Holds
	now           func() time.Time
}

// Deps are the core services the Edition is built over, each the one the composition root
// built for the rest of the instance.
type Deps struct {
	// Client is the raw ent client of the cross-Workspace reads and writes.
	Client *ent.Client
	// Bus is the domain-event bus every write of the Edition goes through. It is built
	// before the Edition, over clients that carry the Edition's ent hooks (Hooks).
	Bus *events.Bus
	// License is the runtime license key; nil is the plain core.
	License *licensekey.License
	// StaffSessions are the core Staff member's sessions, nil while its sign-in is off.
	StaffSessions *staffmembers.Sessions
	// Suspender suspends a Workspace (ADR 0007).
	Suspender *suspension.Suspender
	// Workspaces reads a Workspace by its id and writes the abuse detector's markers on its
	// standing (ADR 0168).
	Workspaces *workspaces.Module
	// Reputation computes a Workspace's rates, which the console shows and the abuse detector
	// judges.
	Reputation *reputation.Module
	// Cipher seals the hashes the Erasures stream stores (ADR 0045).
	Cipher *secrets.Cipher
}

// New builds the Edition over deps. The Enterprise sections of the Staff console take the core
// Staff member's sessions, so the `staff` feature refuses to start without them.
func New(deps Deps, staffCfg workspaceoversight.Config) (*Edition, error) {
	lic, staffSessions, now := deps.License, deps.StaffSessions, staffCfg.Clock
	if now == nil {
		return nil, errors.New("ee: workspaceoversight.Config.Clock is required: the Edition reads the Instance clock")
	}
	auditLog := auditentries.NewLog(deps.Client, lic, deps.Bus)
	usage := usages.New(deps.Client, lic, now)
	e := &Edition{
		lic:        lic,
		client:     deps.Client,
		bus:        deps.Bus,
		suspend:    deps.Suspender,
		workspaces: deps.Workspaces,
		now:        now,
		Audit:      auditLog,
		Erasures:   erasures.New(deps.Client, lic, deps.Cipher, now),
		Usage:      usage,
		Export:     &ExportStreams{lic: lic},
		Staff:      workspaceoversight.New(deps.Client, deps.Bus, lic, auditLog, usage, deps.Suspender, deps.Workspaces, deps.Reputation, staffCfg),

		ServiceCredentials: servicecredentials.New(deps.Client, lic, now),
	}
	e.holds = holds.New(e.HoldsHonoured, staffCfg.HoldMaxExpiry, now)
	if lic.Has(licensekey.FeatureStaff) {
		if staffSessions == nil {
			return nil, errors.New("SPHERICON_STAFF_JWT_SECRET is required with the staff license: the console's Enterprise sections need the Staff member's sign-in")
		}
		e.staffSessions = staffSessions
	}
	return e, nil
}

// Hooks are the Edition's ent hooks for a license and a clock, which internal/app/entclient
// installs with the core's (entclient.WithEdition): hooks on a client bound to a transaction,
// and guards, which refuse the change a hook would record, on any other. The one pair keeps the
// contacts peak of Usage as Contacts change (ADR 0009). Each hook is a no-op without the
// `staff` license.
func Hooks(lic *licensekey.License, now func() time.Time) (hooks, guards []entgo.Hook) {
	return []entgo.Hook{usages.Hook(lic, now)}, []entgo.Hook{usages.Guard(lic)}
}

// SecretSweeps are the Edition's columns of stored secrets, which the Secret re-encryption sweeps
// with the core's: the Erasures' sealed hashes (ADR 0045), whatever the License key, since a row
// stored under a license outlives it.
func (e *Edition) SecretSweeps() []secrets.Sweep { return erasures.SecretSweeps(e.client) }

// StaffSections is the Enterprise part of the /staff API (ADR 0026, ADR 0063) for the
// composition root to mount behind the core one, or nil without the `staff` license, in
// which case its routes answer 404.
func (e *Edition) StaffSections() interface {
	Server(opts ...staffeeapi.ServerOption) (http.Handler, error)
} {
	if e.staffSessions == nil {
		return nil
	}
	return staffhandlers.NewSurface(e.Staff, e.staffSessions)
}

// Service is the service endpoints of the Staff surface (ADR 0026) for the
// composition root to mount on the internal listener, or nil without the `staff`
// license, in which case no listener is bound and nothing is served.
func (e *Edition) Service() interface {
	Server(opts ...serviceapi.ServerOption) (http.Handler, error)
} {
	if e.staffSessions == nil {
		return nil
	}
	return staffhandlers.NewServiceSurface(e.Staff, e.ServiceCredentials)
}

// Holds is the read side of a Workspace's freezes on the send chokepoint (ADR 0009, 0015).
// It honours Staff holds only with the `staff` license, so an unlicensed instance never
// freezes on a row it could not have written; deletion and suspension always count.
func (e *Edition) Holds() *holds.Holds { return e.holds }

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
// a Workspace's standing. It suspends through the core Suspender and tells owners through
// the system (platform) sender. Unlicensed it does nothing.
func (e *Edition) Abuse() *abusedetector.Detector {
	return abusedetector.NewDetector(e.client, e.bus, e.Staff.Watch(), e.suspend, e.workspaces)
}

// AbuseWatch is the read side of the detector for /site: the Workspace's warning, none
// without the `staff` license.
func (e *Edition) AbuseWatch() abusewarning.Watch { return abuseWatchAdapter{e.Staff.Watch()} }

type abuseWatchAdapter struct{ watch *abusedetector.Watch }

func (w abuseWatchAdapter) Warning(ctx context.Context, s *ent.Scoped) (*abusewarning.Warning, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	b, err := w.watch.Warning(ctx, s, ws)
	if err != nil || b.Tier == abusedetector.TierNone {
		return nil, err
	}
	return &abusewarning.Warning{Domain: b.Domain, Metric: b.Metric, Rate: b.Rate()}, nil
}

// Webhooks gates the webhook dispatcher by license: `audit.entry` is forwarded to
// Webhook endpoints only when the Audit feature is licensed.
func (e *Edition) Webhooks(next events.WebhookDispatcher) events.WebhookDispatcher {
	return auditentries.Forwarder(next, e.lic)
}
