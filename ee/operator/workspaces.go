package operator

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent"
	entworkspace "github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// ErrReasonRequired refuses a suspension with no reason: the owner is told why.
var ErrReasonRequired = errors.New("a suspension needs a reason")

// SuspendWorkspace freezes the Workspace's outbound sending as the Operator, through
// the same core mechanism the command line uses (ADR 0007), then tells its owners via
// the system sender. It reports whether anything changed: an already-suspended
// Workspace is left as it was and its owners are not told twice. A failed notice does
// not undo the suspension; it is logged. A blank reason is ErrReasonRequired.
func (m *Module) SuspendWorkspace(ctx context.Context, bus *events.Bus, sender messaging.EmailSender, id int64, by *ent.Operator, reason string) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(reason) == "" {
		return nil, false, ErrReasonRequired
	}
	if _, err := m.ent.Workspace.Get(ctx, id); err != nil {
		return nil, false, err
	}
	changed, err := suspension.SuspendWorkspace(ctx, bus, id, suspension.Operator(strconv.FormatInt(by.ID, 10)), reason)
	if err != nil {
		return nil, false, err
	}
	if changed {
		if err := jobs.NotifyWorkspaceSuspended(ctx, m.ent, sender, m.support, id); err != nil {
			slog.ErrorContext(ctx, "workspace suspended, but the owner notice could not be sent", "workspace_id", id, "error", err)
		}
	}
	ws, err := m.ent.Workspace.Get(ctx, id)
	return ws, changed, err
}

// UnsuspendWorkspace lifts the Workspace's suspension as the Operator; held sends
// resume on their own. It reports whether the Workspace was suspended.
func (m *Module) UnsuspendWorkspace(ctx context.Context, bus *events.Bus, id int64, by *ent.Operator) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	if _, err := m.ent.Workspace.Get(ctx, id); err != nil {
		return nil, false, err
	}
	changed, err := suspension.UnsuspendWorkspace(ctx, bus, id, suspension.Operator(strconv.FormatInt(by.ID, 10)))
	if err != nil {
		return nil, false, err
	}
	ws, err := m.ent.Workspace.Get(ctx, id)
	return ws, changed, err
}

// Rate is a numerator over a denominator; Rate is nil (undefined) below the volume
// floor.
type Rate = reputation.Rate

// DomainRates are the Complaint and Bounce rates of one Sending domain.
type DomainRates struct {
	SendingDomainID int64
	Domain          string
	Complaint       Rate
	Bounce          Rate
}

// Deliverability is a Workspace's rates per Sending domain and its send volume over
// abuse.Window.
type Deliverability struct {
	Window      time.Duration
	VolumeFloor int
	// SendVolume is the messages sent in the window across all Sending domains
	// (email.sent Events, by their own timestamp).
	SendVolume int
	Domains    []DomainRates
}

// Review narrows the Workspace list to the ones the abuse detector surfaces (ADR 0026).
type Review string

const (
	ReviewNone Review = ""
	// ReviewWarned: the owners were warned and the Workspace is still in the warning tier.
	ReviewWarned Review = "warned"
	// ReviewSystemFrozen: suspended by the detector (actor `system`).
	ReviewSystemFrozen Review = "system_frozen"
)

// ListWorkspaces is one page of every Workspace, newest first, optionally only those
// in a review state and those whose slug contains slug (case-insensitive). It reads the Workspace rows themselves
// through the raw client: the Operator console shows tenant metadata only, never a
// Workspace's Contacts, content or Events, so no Workspace-scoped query is made.
func (m *Module) ListWorkspaces(ctx context.Context, review Review, slug string, p pagination.Params) (pagination.Page[*ent.Workspace], error) {
	if err := m.licensed(); err != nil {
		return pagination.Page[*ent.Workspace]{}, err
	}
	query := func() *ent.WorkspaceQuery {
		q := m.ent.Workspace.Query()
		if slug != "" {
			q = q.Where(entworkspace.SlugContainsFold(slug))
		}
		switch review {
		case ReviewWarned:
			q = q.Where(entworkspace.AbuseWarningNotifiedAtNotNil(), entworkspace.SuspendedAtIsNil(), entworkspace.AbuseExempt(false))
		case ReviewSystemFrozen:
			q = q.Where(entworkspace.SuspendedByKindEQ(entworkspace.SuspendedByKindSystem))
		}
		return q
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Workspace, error) {
			return query().Order(ent.Desc(entworkspace.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// GetWorkspace is one Workspace's metadata row; an ent not-found error when it does
// not exist.
func (m *Module) GetWorkspace(ctx context.Context, id int64) (*ent.Workspace, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	return m.ent.Workspace.Get(ctx, id)
}

// WorkspaceDeliverability is the rates of one Workspace, by the existing reputation
// computation (ADR 0011, no new one), and its send volume. A rate stays undefined
// while its Sending domain sent fewer than the volume floor in the window. The
// reputation module reads through a Workspace scope, so one is built here over the
// Workspace being looked at, for this read and the Audit read (AuditEntries) only.
//
// The send volume is the sum of each Sending domain's bounce-rate denominator, which
// the reputation module defines as the count of the domain's email.sent Events in the
// window: it is exactly the messages sent in the window from the Workspace's Sending
// domains (a send naming no Sending domain of the Workspace belongs to no rate).
func (m *Module) WorkspaceDeliverability(ctx context.Context, w *ent.Workspace) (Deliverability, error) {
	if err := m.licensed(); err != nil {
		return Deliverability{}, err
	}
	floor := m.watch.Limits().VolumeFloor
	rates, err := reputation.New().Rates(ctx, m.ent.Scoped(w.ID), abuse.WindowFor(w, m.now()))
	if err != nil {
		return Deliverability{}, err
	}
	out := Deliverability{Window: abuse.Window, VolumeFloor: floor, Domains: make([]DomainRates, 0, len(rates))}
	for _, r := range rates {
		sent := r.Bounce.Denominator
		out.SendVolume += sent
		d := DomainRates{SendingDomainID: r.Domain.ID, Domain: r.Domain.Domain, Complaint: r.Complaint, Bounce: r.Bounce}
		if sent < floor {
			d.Complaint.Rate, d.Bounce.Rate = nil, nil
		}
		out.Domains = append(out.Domains, d)
	}
	return out, nil
}

// SetAbuseExempt exempts the Workspace from the abuse detector, or polices it again, as
// the Operator (ADR 0026), through the same core mechanism, audited. It reports whether
// the flag changed. The Workspace stays listed with its rates and can still be suspended
// by hand.
func (m *Module) SetAbuseExempt(ctx context.Context, bus *events.Bus, id int64, by *ent.Operator, exempt bool) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	if _, err := m.ent.Workspace.Get(ctx, id); err != nil {
		return nil, false, err
	}
	changed, err := suspension.SetAbuseExempt(ctx, bus, id, suspension.Operator(strconv.FormatInt(by.ID, 10)), exempt)
	if err != nil {
		return nil, false, err
	}
	ws, err := m.ent.Workspace.Get(ctx, id)
	return ws, changed, err
}

// WorkspaceAbuse is the Workspace's warning now, by the detector's own computation:
// none while it is exempt, suspended or unlicensed. The detector reads through a
// Workspace scope, built here over the Workspace being looked at (like the rates).
func (m *Module) WorkspaceAbuse(ctx context.Context, w *ent.Workspace) (abuse.Breach, error) {
	if err := m.licensed(); err != nil {
		return abuse.Breach{}, err
	}
	return m.watch.Warning(ctx, m.ent.Scoped(w.ID), w, m.now())
}
