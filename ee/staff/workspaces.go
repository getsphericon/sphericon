package staff

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent"
	entworkspace "github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/reputation"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// ErrReasonRequired refuses a suspension with no reason: the owner is told why.
var ErrReasonRequired = errors.New("a suspension needs a reason")

// SuspendWorkspace freezes the Workspace's outbound sending as the Staff member, through
// the same core mechanism the command line uses (ADR 0007), then tells its owners via
// the system sender. It reports whether anything changed: an already-suspended
// Workspace is left as it was and its owners are not told twice. A failed notice does
// not undo the suspension; it is logged. A blank reason is ErrReasonRequired.
func (m *Module) SuspendWorkspace(ctx context.Context, bus *events.Bus, sender messaging.EmailSender, publicID string, by *ent.StaffMember, reason string) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(reason) == "" {
		return nil, false, ErrReasonRequired
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, false, err
	}
	id := w.ID
	changed, err := suspension.SuspendWorkspace(ctx, bus, id, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), reason)
	if err != nil {
		return nil, false, err
	}
	if changed {
		if err := suspension.NotifyOwners(ctx, m.ent, accounts.OwnerNotifier(m.ent, sender), m.support, id); err != nil {
			slog.ErrorContext(ctx, "workspace suspended, but the owner notice could not be sent", "workspace_id", id, "error", err)
		}
	}
	ws, err := m.ent.Workspace.Get(ctx, id)
	return ws, changed, err
}

// ErrNoSendCaps refuses a Send cap change on an Instance without Send caps (ADR 0067: only the
// saas Mode has them).
var ErrNoSendCaps = errors.New("staff: this Instance has no Send caps")

// SetSendCap sets the Workspace's Send cap, or removes it when sendCap is nil, as the Staff
// member, recorded in its Audit log (suspension.SetSendCap). It reports whether it changed.
func (m *Module) SetSendCap(ctx context.Context, bus *events.Bus, publicID string, by *ent.StaffMember, sendCap *int) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	if !m.sendCaps {
		return nil, false, ErrNoSendCaps
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, false, err
	}
	changed, err := suspension.SetSendCap(ctx, bus, w.ID, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), sendCap)
	if err != nil {
		return nil, false, err
	}
	ws, err := m.ent.Workspace.Get(ctx, w.ID)
	return ws, changed, err
}

// shown is the Workspace as the console shows it: on an Instance without Send caps, a Send cap
// stored on it (a restored dump, say) applies to nothing and is left out.
func (m *Module) shown(w *ent.Workspace) *ent.Workspace {
	if m.sendCaps || w.SendCap == nil {
		return w
	}
	c := *w
	c.SendCap = nil
	return &c
}

// SentLast24h is the Workspace's sends in the last 24 hours, what its Send cap counts (ADR 0067),
// read through a Workspace scope built over the Workspace being looked at (like the rates); nil
// on an Instance without Send caps.
func (m *Module) SentLast24h(ctx context.Context, w *ent.Workspace) (*int, error) {
	if !m.sendCaps {
		return nil, nil
	}
	sent, err := providerlimit.SentLast24h(ctx, m.ent.Scoped(w.ID), m.now())
	return &sent, err
}

// UnsuspendWorkspace lifts the Workspace's suspension as the Staff member; held sends
// resume on their own. It reports whether the Workspace was suspended.
func (m *Module) UnsuspendWorkspace(ctx context.Context, bus *events.Bus, publicID string, by *ent.StaffMember) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, false, err
	}
	id := w.ID
	changed, err := suspension.UnsuspendWorkspace(ctx, bus, id, suspension.StaffMember(strconv.FormatInt(by.ID, 10)))
	if err != nil {
		return nil, false, err
	}
	ws, err := m.ent.Workspace.Get(ctx, id)
	return ws, changed, err
}

// workspace loads a Workspace row by its id (ADR 0043), the only address the console and the
// service endpoints take. ent.IsNotFound for an unknown id; the id is case-sensitive.
func (m *Module) workspace(ctx context.Context, publicID string) (*ent.Workspace, error) {
	return m.ent.Workspace.Query().Where(entworkspace.PublicID(publicID)).Only(ctx)
}

// publicIDs maps internal Workspace keys to their ids in one query, for the service
// responses that name Workspaces in a list.
func (m *Module) publicIDs(ctx context.Context, ids []int64) (map[int64]string, error) {
	rows, err := m.ent.Workspace.Query().Where(entworkspace.IDIn(ids...)).Select(entworkspace.FieldID, entworkspace.FieldPublicID).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, w := range rows {
		out[w.ID] = w.PublicID
	}
	return out, nil
}

// scopeOver loads the Workspace row and scopes over it: the one place the console and the
// service endpoints turn a Workspace id into a scope (docs/architecture.md, scope list item 7).
// ent.IsNotFound for an unknown Workspace.
func (m *Module) scopeOver(ctx context.Context, publicID string) (*ent.Scoped, error) {
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return m.ent.Scoped(w.ID), nil
}

// ListWorkspaces is one page of every Workspace, newest first, optionally only those
// in a review state and those whose id or name contains search: the name matches
// case-insensitively, the id as a case-sensitive substring (base62 is case-sensitive). It reads the Workspace rows themselves
// through the raw client: the Staff console shows tenant metadata only, never a
// Workspace's Contacts, content or Events, so no Workspace-scoped query is made.
func (m *Module) ListWorkspaces(ctx context.Context, review abuse.Review, search string, p pagination.Params) (pagination.Page[*ent.Workspace], error) {
	if err := m.licensed(); err != nil {
		return pagination.Page[*ent.Workspace]{}, err
	}
	query := func() *ent.WorkspaceQuery {
		q := m.ent.Workspace.Query()
		if search != "" {
			q = q.Where(entworkspace.Or(entworkspace.PublicIDContains(search), entworkspace.NameContainsFold(search)))
		}
		return q.Where(review.Predicate())
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Workspace, error) {
			return query().Order(ent.Desc(entworkspace.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// GetWorkspace is one Workspace's metadata row; an ent not-found error when it does
// not exist.
func (m *Module) GetWorkspace(ctx context.Context, publicID string) (*ent.Workspace, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	return m.workspace(ctx, publicID)
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
func (m *Module) WorkspaceDeliverability(ctx context.Context, w *ent.Workspace) (abuse.Deliverability, error) {
	if err := m.licensed(); err != nil {
		return abuse.Deliverability{}, err
	}
	floor := m.watch.Limits().VolumeFloor
	rates, err := reputation.New().Rates(ctx, m.ent.Scoped(w.ID), abuse.WindowFor(w, m.now()))
	if err != nil {
		return abuse.Deliverability{}, err
	}
	out := abuse.Deliverability{Window: abuse.Window, VolumeFloor: floor, Domains: make([]abuse.DomainRates, 0, len(rates))}
	for _, r := range rates {
		sent := r.Bounce.Denominator
		out.SendVolume += sent
		d := abuse.DomainRates{SendingDomainID: r.Domain.ID, Domain: r.Domain.Domain, Complaint: r.Complaint, Bounce: r.Bounce}
		if sent < floor {
			d.Complaint.Rate, d.Bounce.Rate = nil, nil
		}
		out.Domains = append(out.Domains, d)
	}
	return out, nil
}

// SetAbuseExempt exempts the Workspace from the abuse detector, or polices it again, as
// the Staff member (ADR 0026), through the same core mechanism, audited. It reports whether
// the flag changed. The Workspace stays listed with its rates and can still be suspended
// by hand.
func (m *Module) SetAbuseExempt(ctx context.Context, bus *events.Bus, publicID string, by *ent.StaffMember, exempt bool) (*ent.Workspace, bool, error) {
	if err := m.licensed(); err != nil {
		return nil, false, err
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, false, err
	}
	id := w.ID
	changed, err := suspension.SetAbuseExempt(ctx, bus, id, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), exempt)
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
