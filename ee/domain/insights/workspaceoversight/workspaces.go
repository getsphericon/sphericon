package workspaceoversight

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ent"
	entworkspace "github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var (
	// ErrReasonRequired refuses a suspension with no reason: the owner is told why.
	ErrReasonRequired = validation.RefuseField("reason", "staff: a suspension needs a reason", i18n.ErrorsSuspensionReasonRequired)
	// ErrWorkspaceNotFound is an id no Workspace has, on the console and the service endpoints alike.
	ErrWorkspaceNotFound = validation.NotFound("staff: workspace not found", i18n.ErrorsWorkspaceNotFound)
)

// WorkspaceChange is a Staff member's change of a Workspace: the Workspace as it stands after it,
// and whether anything changed (a repeated change leaves it as it was).
type WorkspaceChange struct {
	Workspace *ent.Workspace
	Changed   bool
}

// changeOf is the WorkspaceChange of the Workspace id after a change: the row as it stands now.
func (m *Module) changeOf(ctx context.Context, id int64, changed bool) (*WorkspaceChange, error) {
	ws, err := m.ent.Workspace.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &WorkspaceChange{Workspace: ws, Changed: changed}, nil
}

// SuspendWorkspace freezes the Workspace's outbound sending as the Staff member, through
// the same Suspender the command line uses (ADR 0007), which tells its owners via the
// system sender. It reports whether anything changed: an already-suspended
// Workspace is left as it was and its owners are not told twice. A failed notice does
// not undo the suspension; it is logged. A blank reason is ErrReasonRequired.
func (m *Module) SuspendWorkspace(ctx context.Context, publicID string, by *ent.StaffMember, reason string) (*WorkspaceChange, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(reason) == "" {
		return nil, ErrReasonRequired
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, err
	}
	id := w.ID
	changed, err := m.suspend.Suspend(ctx, id, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), reason)
	if errors.Is(err, suspension.ErrNoticeNotSent) {
		slog.ErrorContext(ctx, "workspace suspended, but the owner notice could not be sent", "workspace_id", id, "error", err)
	} else if err != nil {
		return nil, err
	}
	return m.changeOf(ctx, id, changed)
}

// UnsuspendWorkspace lifts the Workspace's suspension as the Staff member; held sends
// resume on their own. It reports whether the Workspace was suspended.
func (m *Module) UnsuspendWorkspace(ctx context.Context, publicID string, by *ent.StaffMember) (*WorkspaceChange, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, err
	}
	id := w.ID
	changed, err := suspension.UnsuspendWorkspace(ctx, m.bus, id, suspension.StaffMember(strconv.FormatInt(by.ID, 10)))
	if err != nil {
		return nil, err
	}
	return m.changeOf(ctx, id, changed)
}

// workspace loads a Workspace row by its id (ADR 0043), the only address the console and the
// service endpoints take. ErrWorkspaceNotFound for an unknown id; the id is case-sensitive.
func (m *Module) workspace(ctx context.Context, publicID string) (*ent.Workspace, error) {
	w, err := m.ent.Workspace.Query().Where(entworkspace.PublicID(publicID)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrWorkspaceNotFound
	}
	return w, err
}

// publicIDsOf maps the internal Workspace key of every item (key) to the Workspace's id in one
// query, for the service responses that name Workspaces in a list.
func publicIDsOf[T any](ctx context.Context, m *Module, items []T, key func(T) int64) (map[int64]string, error) {
	ids := lo.Map(items, func(item T, _ int) int64 { return key(item) })
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
// ErrWorkspaceNotFound for an unknown Workspace.
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
func (m *Module) ListWorkspaces(ctx context.Context, review abusedetector.Review, search string, p pagination.Params) (pagination.Page[*ent.Workspace], error) {
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

// GetWorkspace is one Workspace's metadata row; ErrWorkspaceNotFound when it does not
// exist.
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
func (m *Module) WorkspaceDeliverability(ctx context.Context, w *ent.Workspace) (abusedetector.Deliverability, error) {
	if err := m.licensed(); err != nil {
		return abusedetector.Deliverability{}, err
	}
	floor := m.watch.Limits().VolumeFloor
	rates, err := m.rates.Rates(ctx, m.ent.Scoped(w.ID), abusedetector.WindowFor(w, m.now()))
	if err != nil {
		return abusedetector.Deliverability{}, err
	}
	out := abusedetector.Deliverability{Window: abusedetector.Window, VolumeFloor: floor, Domains: make([]abusedetector.DomainRates, 0, len(rates))}
	for _, r := range rates {
		sent := r.Bounce.Denominator
		out.SendVolume += sent
		d := abusedetector.DomainRates{SendingDomainID: r.Domain.ID, Domain: r.Domain.Domain, Complaint: r.Complaint, Bounce: r.Bounce}
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
func (m *Module) SetAbuseExempt(ctx context.Context, publicID string, by *ent.StaffMember, exempt bool) (*WorkspaceChange, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	w, err := m.workspace(ctx, publicID)
	if err != nil {
		return nil, err
	}
	id := w.ID
	changed, err := suspension.SetAbuseExempt(ctx, m.bus, id, suspension.StaffMember(strconv.FormatInt(by.ID, 10)), exempt)
	if err != nil {
		return nil, err
	}
	return m.changeOf(ctx, id, changed)
}

// WorkspaceAbuse is the Workspace's warning now, by the detector's own computation:
// none while it is exempt, suspended or unlicensed. The detector reads through a
// Workspace scope, built here over the Workspace being looked at (like the rates).
func (m *Module) WorkspaceAbuse(ctx context.Context, w *ent.Workspace) (abusedetector.Breach, error) {
	if err := m.licensed(); err != nil {
		return abusedetector.Breach{}, err
	}
	return m.watch.Warning(ctx, m.ent.Scoped(w.ID), w, m.now())
}
