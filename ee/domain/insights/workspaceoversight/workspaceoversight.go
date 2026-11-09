// Package workspaceoversight is the Enterprise domain module of the Workspace oversight (GLOSSARY,
// ADR 0026, ADR 0063, ADR 0102), the Enterprise sections of the Staff console under the `staff`
// license feature: the cross-Workspace views and actions of a Staff member, and what the service
// endpoints a Service credential calls read and write. It is not a use case: each change it makes
// goes through the one module that owns the rows (suspension, holds), and its listings read tenant
// metadata (the Workspace rows, the Users and their tombstones, the Tracking host changes) through the
// raw client, as it did as a use case; routing those reads through their owners is left to their
// owners' modules. What it owns are the rules of looking across
// Workspaces: the `staff` gate on every entry point, the Workspace id as the only address, tenant
// metadata only (never a Workspace's Contacts, content or Events), a suspension's reason, a hold's
// expiry window and its https action URL. It sits at the top of insights and calls the modules
// that own the rows, of its context and below (suspension, holds, the Send cap, the Audit log,
// Usage, the Abuse detector, the service feeds).
// It owns no table: the Service credentials are the domain module
// ee/domain/identity/servicecredentials. Its handlers are ee/api/staff, which call this package
// and hold no logic of their own. The Staff member itself, its sign-in and its sessions are core
// (internal/domain/identity/staffmembers) and run on every Instance.
//
// The console's Workspace metadata belongs to no Workspace, so this package works on the raw
// *ent.Client (it is in the closed list of docs/architecture/workspace-scoping.md): the Workspace rows, the
// suspension state and the rates the console shows, which are tenant metadata rather than a
// Workspace's data. Two reads build a Workspace scope, each over the one Workspace the Staff
// member is looking at, because the module they ride takes one: the Audit log
// (Module.AuditEntries) and the reputation rates (Module.WorkspaceDeliverability). Every entry
// point checks the `staff` license feature.
//
// Governed by ee/LICENSE, not the AGPL.
package workspaceoversight

import (
	"time"

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupsurveyresponses"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/domain/sending/reputation"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// ErrNotLicensed is the refusal on an instance without the `staff` license: the Enterprise
// sections are off. It is the Service credentials' refusal, so one sentinel answers for both.
var ErrNotLicensed = servicecredentials.ErrNotLicensed

// Config is what the Enterprise sections need from the instance.
type Config struct {
	// Clock is the Instance clock of hold expiries and the console's rate windows. Required.
	Clock func() time.Time
	// Abuse are the abuse detector's limits (ADR 0026); the console shows rates under
	// the same volume floor the detector judges by. Zero is abusedetector.DefaultLimits.
	Abuse abusedetector.Limits
	// SupportEmail is the support contact an owner is given with the abuse detector's
	// warning (SPHERICON_SUPPORT_EMAIL).
	SupportEmail string
	// HoldMaxExpiry is how far ahead a Staff hold's expiry may be set
	// (SPHERICON_STAFF_HOLD_MAX_EXPIRY); zero is DefaultHoldMaxExpiry.
	HoldMaxExpiry time.Duration
	// SendCaps is whether the Instance has Send caps (ADR 0067). Without them the console shows
	// no Send cap and its endpoints are not found, whatever the License.
	SendCaps providerlimit.SendCaps
}

// Module is the console's Enterprise sections and the service endpoints.
type Module struct {
	ent      *ent.Client
	bus      *events.Bus
	suspend  *suspension.Suspender
	rates    *reputation.Module
	lic      *licensekey.License
	audit    *auditentries.Log
	now      func() time.Time
	watch    *abusedetector.Watch
	holdMax  time.Duration
	survey   *signupsurveyresponses.Reader
	sendCaps providerlimit.SendCaps
}

// New builds the module over the raw client.
func New(client *ent.Client, bus *events.Bus, lic *licensekey.License, auditLog *auditentries.Log, suspender *suspension.Suspender, rates *reputation.Module, cfg Config) *Module {
	now := cfg.Clock
	if now == nil {
		panic("workspaceoversight: Config.Clock is required: the module reads the Instance clock")
	}
	holdMax := cfg.HoldMaxExpiry
	if holdMax <= 0 {
		holdMax = DefaultHoldMaxExpiry
	}
	return &Module{
		ent: client, bus: bus, suspend: suspender, rates: rates, lic: lic, audit: auditLog, now: now, watch: abusedetector.NewWatch(lic, cfg.Abuse), holdMax: holdMax, survey: signupsurveyresponses.NewReader(client),
		sendCaps: cfg.SendCaps,
	}
}

// Watch is the abuse detector's read of a Workspace's standing, with the limits this
// module shows rates under.
func (m *Module) Watch() *abusedetector.Watch { return m.watch }

func (m *Module) licensed() error {
	if !m.lic.Has(licensekey.FeatureStaff) {
		return ErrNotLicensed
	}
	return nil
}
