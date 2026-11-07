// Package staff is the handlers of the Enterprise Staff surfaces (ADR 0026, ADR 0063): the Staff
// console's Enterprise sections under /staff, with the core Staff member's session check
// (internal/domain/identity/staffmembers), and the service endpoints under /service, with the Service credential's.
// A handler receives the request, calls the module ee/domain/insights/workspaceoversight and maps what it returns; the
// Edition (ee) builds the surfaces and the composition root mounts them.
//
// Governed by ee/LICENSE, not the AGPL.
package staff

import (
	"context"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	"github.com/getsphericon/sphericon/ent"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/api/problem"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
)

// Surface is the Enterprise part of the /staff API: the handlers and the security handler
// over one Module, for the composition root to mount behind the core /staff surface. The
// session is the core Staff member's (internal/domain/identity/staffmembers).
type Surface struct {
	handlers *Handlers
	security *SecurityHandler
}

// NewSurface builds the surface.
func NewSurface(m *workspaceoversight.Module, s *staffmembers.Sessions) *Surface {
	return &Surface{handlers: &Handlers{module: m}, security: &SecurityHandler{sessions: s}}
}

// Server builds the ogen server under the /staff prefix. opts carry what the
// composition root owns (the RFC 7807 error handler).
func (s *Surface) Server(opts ...staffeeapi.ServerOption) (http.Handler, error) {
	return staffeeapi.NewServer(s.handlers, s.security,
		append([]staffeeapi.ServerOption{staffeeapi.WithPathPrefix(paths.Staff)}, opts...)...)
}

// Handlers implements staffeeapi.Handler.
type Handlers struct {
	module *workspaceoversight.Module
}

var _ staffeeapi.Handler = (*Handlers)(nil)

// NewError answers a refused or failed request with the contract's Problem: the status and
// the body are decided once for every surface (internal/api/problem).
func (*Handlers) NewError(ctx context.Context, err error) *staffeeapi.ProblemDetailsStatusCode {
	return problem.Staffee(ctx, err)
}

// SecurityHandler implements staffeeapi.SecurityHandler: the core Staff member cookie.
type SecurityHandler struct{ sessions *staffmembers.Sessions }

var _ staffeeapi.SecurityHandler = (*SecurityHandler)(nil)

// HandleApiKeyAuth authenticates the Staff member cookie and puts the Staff member in the
// context.
func (h *SecurityHandler) HandleApiKeyAuth(ctx context.Context, _ staffeeapi.OperationName, t staffeeapi.ApiKeyAuth) (context.Context, error) {
	return h.sessions.Authenticate(ctx, t.APIKey)
}

// StaffWorkspacesList is a page of Workspaces, newest first, narrowed by an id or name
// search. Metadata only: no Contacts, content or Events.
func (h *Handlers) StaffWorkspacesList(ctx context.Context, params staffeeapi.StaffWorkspacesListParams) (*staffeeapi.StaffWorkspacesListOK, error) {
	page, err := h.module.ListWorkspaces(ctx, review(params.Review), strings.TrimSpace(params.Search.Value), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return staffMapper.WorkspacesListPage(page), nil
}

// review is the review state the list is filtered by; none narrows it to no state.
func review(p staffeeapi.OptStaffWorkspaceReview) abusedetector.Review {
	v, ok := p.Get()
	if !ok {
		return abusedetector.ReviewNone
	}
	return staffMapper.ReviewIn(v)
}

// StaffWorkspacesGet is one Workspace's metadata and suspension state.
func (h *Handlers) StaffWorkspacesGet(ctx context.Context, params staffeeapi.StaffWorkspacesGetParams) (*staffeeapi.StaffWorkspaceDetailResource, error) {
	w, err := h.module.GetWorkspace(ctx, string(params.WorkspaceId))
	if err != nil {
		return nil, err
	}
	d, err := h.module.WorkspaceDeliverability(ctx, w)
	if err != nil {
		return nil, err
	}
	active, err := h.module.ActiveHolds(ctx, w.PublicID)
	if err != nil {
		return nil, err
	}
	breach, err := h.module.WorkspaceAbuse(ctx, w)
	if err != nil {
		return nil, err
	}
	res := staffMapper.WorkspaceDetailToResource(w)
	res.Holds = staffMapper.HoldsToResources(active)
	res.Abuse = staffMapper.AbuseToResource(w)
	if breach.Tier != abusedetector.TierNone {
		res.Abuse.Warning = staffeeapi.NewOptNilStaffAbuseWarning(staffMapper.BreachToWarning(breach))
	}
	res.Deliverability = staffMapper.DeliverabilityToResource(d)
	if h.module.SendCaps() {
		sendCap, err := h.sendCap(ctx, w)
		if err != nil {
			return nil, err
		}
		res.SendCap = staffeeapi.NewOptStaffSendCap(sendCap)
	}
	return &res, nil
}

// StaffWorkspacesSetSendCap sets a Workspace's Send cap as the signed-in Staff member.
func (h *Handlers) StaffWorkspacesSetSendCap(ctx context.Context, req *staffeeapi.StaffSendCapInput, params staffeeapi.StaffWorkspacesSetSendCapParams) (*staffeeapi.StaffSendCapChange, error) {
	perDay := int(req.PerDay)
	return h.setSendCap(ctx, params.WorkspaceId, &perDay)
}

// StaffWorkspacesClearSendCap removes a Workspace's Send cap as the signed-in Staff member.
func (h *Handlers) StaffWorkspacesClearSendCap(ctx context.Context, params staffeeapi.StaffWorkspacesClearSendCapParams) (*staffeeapi.StaffSendCapChange, error) {
	return h.setSendCap(ctx, params.WorkspaceId, nil)
}

// setSendCap is the change both Send cap operations make.
func (h *Handlers) setSendCap(ctx context.Context, wsID staffeeapi.WorkspaceId, perDay *int) (*staffeeapi.StaffSendCapChange, error) {
	w, changed, err := h.module.SetSendCap(ctx, string(wsID), staffmembers.Current(ctx), perDay)
	if err != nil {
		return nil, err
	}
	sendCap, err := h.sendCap(ctx, w)
	if err != nil {
		return nil, err
	}
	return &staffeeapi.StaffSendCapChange{Changed: changed, SendCap: sendCap}, nil
}

// sendCap is the Workspace's Send cap with its sends over the last 24 hours.
func (h *Handlers) sendCap(ctx context.Context, w *ent.Workspace) (staffeeapi.StaffSendCap, error) {
	sent, err := h.module.SentLast24h(ctx, w)
	if err != nil {
		return staffeeapi.StaffSendCap{}, err
	}
	res := staffMapper.SendCapToResource(w)
	res.SentLast24h = narrow.Int32(sent)
	return res, nil
}

func change(w staffeeapi.StaffWorkspaceResource, changed bool) *staffeeapi.StaffSuspensionChange {
	return &staffeeapi.StaffSuspensionChange{Changed: changed, Workspace: w}
}

// StaffWorkspacesSuspend freezes a Workspace's outbound sending as the signed-in
// Staff member, whose id the audit trail keeps while customers see "Sphericon staff".
func (h *Handlers) StaffWorkspacesSuspend(ctx context.Context, req *staffeeapi.StaffSuspendInput, params staffeeapi.StaffWorkspacesSuspendParams) (*staffeeapi.StaffSuspensionChange, error) {
	w, changed, err := h.module.SuspendWorkspace(ctx, string(params.WorkspaceId), staffmembers.Current(ctx), req.Reason)
	if err != nil {
		return nil, err
	}
	return change(staffMapper.WorkspaceToResource(w), changed), nil
}

// StaffWorkspacesUnsuspend lifts a Workspace's suspension as the signed-in Staff member.
func (h *Handlers) StaffWorkspacesUnsuspend(ctx context.Context, params staffeeapi.StaffWorkspacesUnsuspendParams) (*staffeeapi.StaffSuspensionChange, error) {
	w, changed, err := h.module.UnsuspendWorkspace(ctx, string(params.WorkspaceId), staffmembers.Current(ctx))
	if err != nil {
		return nil, err
	}
	return change(staffMapper.WorkspaceToResource(w), changed), nil
}

// StaffWorkspacesExempt exempts a Workspace from the abuse detector as the signed-in
// Staff member.
func (h *Handlers) StaffWorkspacesExempt(ctx context.Context, params staffeeapi.StaffWorkspacesExemptParams) (*staffeeapi.StaffExemptionChange, error) {
	return h.setExempt(ctx, params.WorkspaceId, true)
}

// StaffWorkspacesUnexempt puts a Workspace back under the abuse detector.
func (h *Handlers) StaffWorkspacesUnexempt(ctx context.Context, params staffeeapi.StaffWorkspacesUnexemptParams) (*staffeeapi.StaffExemptionChange, error) {
	return h.setExempt(ctx, params.WorkspaceId, false)
}

// setExempt is the change both exemption operations make.
func (h *Handlers) setExempt(ctx context.Context, wsID staffeeapi.WorkspaceId, exempt bool) (*staffeeapi.StaffExemptionChange, error) {
	w, changed, err := h.module.SetAbuseExempt(ctx, string(wsID), staffmembers.Current(ctx), exempt)
	if err != nil {
		return nil, err
	}
	return &staffeeapi.StaffExemptionChange{Changed: changed, Workspace: staffMapper.WorkspaceToResource(w)}, nil
}
