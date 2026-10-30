// Package staff holds the Enterprise sections of the Staff console (ADR 0026, ADR 0063), under
// the `staff` license feature: the cross-Workspace views and actions of a Staff member, and the
// service endpoints a Service credential calls. The Staff member itself, its sign-in and its
// sessions are core (internal/staffmembers) and run on every Instance; this package takes the
// session check from there.
//
// The console's Workspace metadata belongs to no Workspace, so this package works on the raw
// *ent.Client (it is in the closed list of docs/architecture.md): the Workspace rows, the
// suspension state and the rates the console shows, which are tenant metadata rather than a
// Workspace's data. Two reads build a Workspace scope, each over the one Workspace the Staff
// member is looking at, because the module they ride takes one: the Audit log
// (Module.AuditEntries) and the reputation rates (Module.WorkspaceDeliverability). Every entry
// point checks the `staff` license feature.
//
// Governed by ee/LICENSE, not the AGPL.
package staff

import (
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/audit"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/signupsurvey"
)

// ErrNotLicensed is the refusal on an instance without the `staff` license: the Enterprise
// sections are off.
var ErrNotLicensed = errors.New("staff: the staff feature is not licensed")

// Config is what the Enterprise sections need from the instance.
type Config struct {
	// Clock is the clock of hold expiries and the console's rate windows; nil is time.Now.
	Clock func() time.Time
	// Abuse are the abuse detector's limits (ADR 0026); the console shows rates under
	// the same volume floor the detector judges by. Zero is abuse.DefaultLimits.
	Abuse abuse.Limits
	// SupportEmail is the support contact an owner is given with a suspension notice
	// (SPHERICON_SUPPORT_EMAIL).
	SupportEmail string
	// HoldMaxExpiry is how far ahead a Staff hold's expiry may be set
	// (SPHERICON_STAFF_HOLD_MAX_EXPIRY); zero is DefaultHoldMaxExpiry.
	HoldMaxExpiry time.Duration
}

// Module is the domain of the console's Enterprise sections and of the service endpoints.
type Module struct {
	ent     *ent.Client
	lic     *licensekey.License
	audit   *audit.Log
	now     func() time.Time
	watch   *abuse.Watch
	support string
	holdMax time.Duration
	survey  *signupsurvey.Reader
}

// NewModule builds the module over the raw client.
func NewModule(client *ent.Client, lic *licensekey.License, auditLog *audit.Log, cfg Config) *Module {
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	holdMax := cfg.HoldMaxExpiry
	if holdMax <= 0 {
		holdMax = DefaultHoldMaxExpiry
	}
	return &Module{
		ent: client, lic: lic, audit: auditLog, now: now, watch: abuse.NewWatch(lic, cfg.Abuse), support: cfg.SupportEmail, holdMax: holdMax, survey: signupsurvey.NewReader(client),
	}
}

// Watch is the abuse detector's read of a Workspace's standing, with the limits this
// module shows rates under.
func (m *Module) Watch() *abuse.Watch { return m.watch }

func (m *Module) licensed() error {
	if !m.lic.Has(licensekey.FeatureStaff) {
		return ErrNotLicensed
	}
	return nil
}
