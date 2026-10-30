package staff

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/getsphericon/sphericon/internal/staffmembers"
	"github.com/samber/lo"
)

// Surface is the Enterprise part of the /staff API: the handlers and the security handler
// over one Module, for the composition root to mount behind the core /staff surface. The
// session is the core Staff member's (internal/staffmembers).
type Surface struct {
	handlers *Handlers
	security *SecurityHandler
}

// NewSurface builds the surface.
func NewSurface(m *Module, s *staffmembers.Sessions, bus *events.Bus, sender messaging.EmailSender) *Surface {
	return &Surface{handlers: &Handlers{module: m, bus: bus, sender: sender}, security: &SecurityHandler{sessions: s}}
}

// Server builds the ogen server under the /staff prefix. opts carry what the
// composition root owns (the RFC 7807 error handler).
func (s *Surface) Server(opts ...staffeeapi.ServerOption) (http.Handler, error) {
	return staffeeapi.NewServer(s.handlers, s.security,
		append([]staffeeapi.ServerOption{staffeeapi.WithPathPrefix(paths.Staff)}, opts...)...)
}

// Handlers implements staffeeapi.Handler.
type Handlers struct {
	module *Module
	bus    *events.Bus
	sender messaging.EmailSender // the system (platform) sender that tells a suspended Workspace's owner
}

var _ staffeeapi.Handler = (*Handlers)(nil)

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
func (h *Handlers) StaffWorkspacesList(ctx context.Context, params staffeeapi.StaffWorkspacesListParams) (staffeeapi.StaffWorkspacesListRes, error) {
	page, err := h.module.ListWorkspaces(ctx, review(params.Review), strings.TrimSpace(params.Search.Value), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]staffeeapi.StaffWorkspaceResource, 0, len(page.Items))
	for _, w := range page.Items {
		items = append(items, staffMapper.WorkspaceToResource(w))
	}
	return &staffeeapi.StaffWorkspacesListOK{
		Items:      items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func review(p staffeeapi.OptStaffWorkspaceReview) abuse.Review {
	switch p.Value {
	case staffeeapi.StaffWorkspaceReviewWarned:
		return abuse.ReviewWarned
	case staffeeapi.StaffWorkspaceReviewSystemFrozen:
		return abuse.ReviewSystemFrozen
	case staffeeapi.StaffWorkspaceReviewDeleted:
		return abuse.ReviewDeleted
	default:
		return abuse.ReviewNone
	}
}

// StaffWorkspacesGet is one Workspace's metadata and suspension state.
func (h *Handlers) StaffWorkspacesGet(ctx context.Context, params staffeeapi.StaffWorkspacesGetParams) (staffeeapi.StaffWorkspacesGetRes, error) {
	id := string(params.WorkspaceId)
	w, err := h.module.GetWorkspace(ctx, id)
	if ent.IsNotFound(err) {
		return workspaceNotFound(), nil
	}
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
	res.Holds = lo.Map(active, func(h *ent.Hold, _ int) staffeeapi.StaffHold { return staffMapper.HoldToResource(h) })
	res.Abuse = staffMapper.AbuseToResource(w)
	if breach.Tier != abuse.TierNone {
		res.Abuse.Warning = staffeeapi.NewOptNilStaffAbuseWarning(staffMapper.BreachToWarning(breach))
	}
	res.Deliverability = staffMapper.DeliverabilityToResource(d)
	return &res, nil
}

func workspaceNotFound() *staffeeapi.StaffWorkspacesGetNotFound {
	p := staffeeapi.StaffWorkspacesGetNotFound(notFoundProblem())
	return &p
}

func notFoundProblem() staffeeapi.ProblemDetails {
	return staffeeapi.ProblemDetails{
		Status: staffeeapi.NewOptInt32(http.StatusNotFound),
		Title:  staffeeapi.NewOptString(http.StatusText(http.StatusNotFound)),
		Detail: staffeeapi.NewOptString(i18n.T("errors.workspace_not_found", nil)),
	}
}

func change(w staffeeapi.StaffWorkspaceResource, changed bool) *staffeeapi.StaffSuspensionChange {
	return &staffeeapi.StaffSuspensionChange{Changed: changed, Workspace: w}
}

// StaffWorkspacesSuspend freezes a Workspace's outbound sending as the signed-in
// Staff member, whose id the audit trail keeps while customers see "Sphericon staff".
func (h *Handlers) StaffWorkspacesSuspend(ctx context.Context, req *staffeeapi.StaffSuspendInput, params staffeeapi.StaffWorkspacesSuspendParams) (staffeeapi.StaffWorkspacesSuspendRes, error) {
	id := string(params.WorkspaceId)
	w, changed, err := h.module.SuspendWorkspace(ctx, h.bus, h.sender, id, staffmembers.Current(ctx), req.Reason)
	if errors.Is(err, ErrReasonRequired) {
		p := staffeeapi.StaffWorkspacesSuspendUnprocessableEntity{
			Status: staffeeapi.NewOptInt32(http.StatusUnprocessableEntity),
			Title:  staffeeapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
			Detail: staffeeapi.NewOptString(i18n.T("errors.suspension_reason_required", nil)),
		}
		return &p, nil
	}
	if ent.IsNotFound(err) {
		p := staffeeapi.StaffWorkspacesSuspendNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return change(staffMapper.WorkspaceToResource(w), changed), nil
}

// StaffWorkspacesUnsuspend lifts a Workspace's suspension as the signed-in Staff member.
func (h *Handlers) StaffWorkspacesUnsuspend(ctx context.Context, params staffeeapi.StaffWorkspacesUnsuspendParams) (staffeeapi.StaffWorkspacesUnsuspendRes, error) {
	id := string(params.WorkspaceId)
	w, changed, err := h.module.UnsuspendWorkspace(ctx, h.bus, id, staffmembers.Current(ctx))
	if ent.IsNotFound(err) {
		p := staffeeapi.StaffWorkspacesUnsuspendNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return change(staffMapper.WorkspaceToResource(w), changed), nil
}

// StaffWorkspacesExempt exempts a Workspace from the abuse detector as the signed-in
// Staff member.
func (h *Handlers) StaffWorkspacesExempt(ctx context.Context, params staffeeapi.StaffWorkspacesExemptParams) (staffeeapi.StaffWorkspacesExemptRes, error) {
	w, changed, notFound, err := h.setExempt(ctx, params.WorkspaceId, true)
	if notFound {
		p := staffeeapi.StaffWorkspacesExemptNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &staffeeapi.StaffExemptionChange{Changed: changed, Workspace: staffMapper.WorkspaceToResource(w)}, nil
}

// StaffWorkspacesUnexempt puts a Workspace back under the abuse detector.
func (h *Handlers) StaffWorkspacesUnexempt(ctx context.Context, params staffeeapi.StaffWorkspacesUnexemptParams) (staffeeapi.StaffWorkspacesUnexemptRes, error) {
	w, changed, notFound, err := h.setExempt(ctx, params.WorkspaceId, false)
	if notFound {
		p := staffeeapi.StaffWorkspacesUnexemptNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &staffeeapi.StaffExemptionChange{Changed: changed, Workspace: staffMapper.WorkspaceToResource(w)}, nil
}

func (h *Handlers) setExempt(ctx context.Context, wsID staffeeapi.WorkspaceId, exempt bool) (w *ent.Workspace, changed, notFound bool, err error) {
	id := string(wsID)
	w, changed, err = h.module.SetAbuseExempt(ctx, h.bus, id, staffmembers.Current(ctx), exempt)
	if ent.IsNotFound(err) {
		return nil, false, true, nil
	}
	return w, changed, false, err
}
