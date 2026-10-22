package staff

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/samber/lo"
)

// Surface is the /staff API: the handlers and the security handler over one
// Module, for the composition root to mount.
type Surface struct {
	handlers *Handlers
	security *SecurityHandler
}

// NewSurface builds the surface.
func NewSurface(m *Module, s *Sessions, bus *events.Bus, sender messaging.EmailSender) *Surface {
	return &Surface{handlers: &Handlers{module: m, sessions: s, bus: bus, sender: sender}, security: &SecurityHandler{sessions: s}}
}

// Server builds the ogen server under the /staff prefix. opts carry what the
// composition root owns (the RFC 7807 error handler).
func (s *Surface) Server(opts ...staffapi.ServerOption) (http.Handler, error) {
	return staffapi.NewServer(s.handlers, s.security,
		append([]staffapi.ServerOption{staffapi.WithPathPrefix(paths.Staff)}, opts...)...)
}

// Handlers implements staffapi.Handler.
type Handlers struct {
	module   *Module
	sessions *Sessions
	bus      *events.Bus
	sender   messaging.EmailSender // the system (platform) sender that tells a suspended Workspace's owner
}

var _ staffapi.Handler = (*Handlers)(nil)

// StaffAuthLogin is the password step, run by the login module: it answers a challenge
// (or, at first login, an enrolment) and never a session. An unknown email and a wrong
// password answer the same 401.
func (h *Handlers) StaffAuthLogin(ctx context.Context, req *staffapi.StaffLoginInput) (staffapi.StaffAuthLoginRes, error) {
	res, err := h.module.CheckPassword(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.Throttled:
		return nil, throttled(ctx, res.Delay)
	case login.InvalidCredentials:
		return unauthorized(i18n.T("errors.invalid_credentials", nil)), nil
	case login.SecondFactor:
		out := &staffapi.StaffLoginResult{Outcome: staffapi.StaffLoginOutcomeChallenge, Challenge: res.Challenge}
		key, err := h.module.Enrolment(ctx, res.Principal)
		if err != nil {
			return nil, err
		}
		if key != nil {
			out.Outcome = staffapi.StaffLoginOutcomeEnrolment
			out.Enrolment = staffapi.NewOptStaffEnrolment(staffapi.StaffEnrolment{Secret: key.Secret, URI: key.URI, QrCode: key.QRCode})
		}
		return out, nil
	case login.Gated, login.Authenticated:
		// The Staff login has no gate and always has a second step.
	}
	return nil, fmt.Errorf("staff: unhandled login outcome %d", res.Outcome)
}

// StaffAuthSecondFactor is the second step: a valid challenge and a current TOTP
// code start the session. A bad challenge and a wrong code answer the same 401.
func (h *Handlers) StaffAuthSecondFactor(ctx context.Context, req *staffapi.StaffSecondFactorInput) (staffapi.StaffAuthSecondFactorRes, error) {
	res, err := h.module.Complete(ctx, req.Challenge, req.Code)
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.SecondThrottled:
		return nil, throttled(ctx, res.Delay)
	case login.SecondInvalidChallenge:
		return unauthorized(i18n.T("errors.login_challenge_invalid", nil)), nil
	case login.SecondInvalidCode:
		return unauthorized(i18n.T("errors.second_factor_code_invalid", nil)), nil
	case login.SecondAuthenticated:
		// No audit entry: the Staff member belongs to no Workspace.
		cookie, err := h.sessions.Issue(res.Principal)
		if err != nil {
			return nil, err
		}
		return &staffapi.StaffResourceHeaders{SetCookie: cookie.String(), Response: staffMapper.StaffMemberToResource(res.Principal)}, nil
	}
	return nil, fmt.Errorf("staff: unhandled login outcome %d", res.Outcome)
}

// StaffAuthLogout ends the Staff member's sessions server-side (the session epoch is
// bumped, so a token copied elsewhere stops verifying too) and clears the cookie.
func (h *Handlers) StaffAuthLogout(ctx context.Context) (staffapi.StaffAuthLogoutRes, error) {
	if err := h.module.EndSessions(ctx, Current(ctx).ID); err != nil {
		return nil, err
	}
	return &staffapi.StaffAuthLogoutNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// StaffMeGet is the current Staff member.
func (h *Handlers) StaffMeGet(ctx context.Context) (staffapi.StaffMeGetRes, error) {
	res := staffMapper.StaffMemberToResource(Current(ctx))
	return &res, nil
}

// throttled is the login throttle's refusal as the standard 429 (ADR 0025): the
// Retry-After and X-RateLimit headers are set on the response and the error handler
// renders the problem.
func throttled(ctx context.Context, res login.Delay) error {
	return ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, res.Limit, res.Wait, res.Now)
}

// problem is the RFC 7807 body of a refusal.
type problem = staffapi.ProblemDetails

// unauthorized is the 401 of a failed login step. It satisfies both login responses.
func unauthorized(detail string) *problem {
	return &problem{
		Status: staffapi.NewOptInt32(http.StatusUnauthorized),
		Title:  staffapi.NewOptString(http.StatusText(http.StatusUnauthorized)),
		Detail: staffapi.NewOptString(detail),
	}
}

// StaffWorkspacesList is a page of Workspaces, newest first, narrowed by an id or name
// search. Metadata only: no Contacts, content or Events.
func (h *Handlers) StaffWorkspacesList(ctx context.Context, params staffapi.StaffWorkspacesListParams) (staffapi.StaffWorkspacesListRes, error) {
	page, err := h.module.ListWorkspaces(ctx, review(params.Review), strings.TrimSpace(params.Search.Value), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]staffapi.StaffWorkspaceResource, 0, len(page.Items))
	for _, w := range page.Items {
		items = append(items, staffMapper.WorkspaceToResource(w))
	}
	return &staffapi.StaffWorkspacesListOK{
		Items:      items,
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func review(p staffapi.OptStaffWorkspaceReview) abuse.Review {
	switch p.Value {
	case staffapi.StaffWorkspaceReviewWarned:
		return abuse.ReviewWarned
	case staffapi.StaffWorkspaceReviewSystemFrozen:
		return abuse.ReviewSystemFrozen
	case staffapi.StaffWorkspaceReviewDeleted:
		return abuse.ReviewDeleted
	default:
		return abuse.ReviewNone
	}
}

// StaffWorkspacesGet is one Workspace's metadata and suspension state.
func (h *Handlers) StaffWorkspacesGet(ctx context.Context, params staffapi.StaffWorkspacesGetParams) (staffapi.StaffWorkspacesGetRes, error) {
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
	res.Holds = lo.Map(active, func(h *ent.Hold, _ int) staffapi.StaffHold { return staffMapper.HoldToResource(h) })
	res.Abuse = staffMapper.AbuseToResource(w)
	if breach.Tier != abuse.TierNone {
		res.Abuse.Warning = staffapi.NewOptNilStaffAbuseWarning(staffMapper.BreachToWarning(breach))
	}
	res.Deliverability = staffMapper.DeliverabilityToResource(d)
	return &res, nil
}

func workspaceNotFound() *staffapi.StaffWorkspacesGetNotFound {
	p := staffapi.StaffWorkspacesGetNotFound(notFoundProblem())
	return &p
}

func notFoundProblem() staffapi.ProblemDetails {
	return staffapi.ProblemDetails{
		Status: staffapi.NewOptInt32(http.StatusNotFound),
		Title:  staffapi.NewOptString(http.StatusText(http.StatusNotFound)),
		Detail: staffapi.NewOptString(i18n.T("errors.workspace_not_found", nil)),
	}
}

func change(w staffapi.StaffWorkspaceResource, changed bool) *staffapi.StaffSuspensionChange {
	return &staffapi.StaffSuspensionChange{Changed: changed, Workspace: w}
}

// StaffWorkspacesSuspend freezes a Workspace's outbound sending as the signed-in
// Staff member, whose id the audit trail keeps while customers see "Sphericon staff".
func (h *Handlers) StaffWorkspacesSuspend(ctx context.Context, req *staffapi.StaffSuspendInput, params staffapi.StaffWorkspacesSuspendParams) (staffapi.StaffWorkspacesSuspendRes, error) {
	id := string(params.WorkspaceId)
	w, changed, err := h.module.SuspendWorkspace(ctx, h.bus, h.sender, id, Current(ctx), req.Reason)
	if errors.Is(err, ErrReasonRequired) {
		p := staffapi.StaffWorkspacesSuspendUnprocessableEntity{
			Status: staffapi.NewOptInt32(http.StatusUnprocessableEntity),
			Title:  staffapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
			Detail: staffapi.NewOptString(i18n.T("errors.suspension_reason_required", nil)),
		}
		return &p, nil
	}
	if ent.IsNotFound(err) {
		p := staffapi.StaffWorkspacesSuspendNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return change(staffMapper.WorkspaceToResource(w), changed), nil
}

// StaffWorkspacesUnsuspend lifts a Workspace's suspension as the signed-in Staff member.
func (h *Handlers) StaffWorkspacesUnsuspend(ctx context.Context, params staffapi.StaffWorkspacesUnsuspendParams) (staffapi.StaffWorkspacesUnsuspendRes, error) {
	id := string(params.WorkspaceId)
	w, changed, err := h.module.UnsuspendWorkspace(ctx, h.bus, id, Current(ctx))
	if ent.IsNotFound(err) {
		p := staffapi.StaffWorkspacesUnsuspendNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return change(staffMapper.WorkspaceToResource(w), changed), nil
}

// StaffWorkspacesExempt exempts a Workspace from the abuse detector as the signed-in
// Staff member.
func (h *Handlers) StaffWorkspacesExempt(ctx context.Context, params staffapi.StaffWorkspacesExemptParams) (staffapi.StaffWorkspacesExemptRes, error) {
	w, changed, notFound, err := h.setExempt(ctx, params.WorkspaceId, true)
	if notFound {
		p := staffapi.StaffWorkspacesExemptNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &staffapi.StaffExemptionChange{Changed: changed, Workspace: staffMapper.WorkspaceToResource(w)}, nil
}

// StaffWorkspacesUnexempt puts a Workspace back under the abuse detector.
func (h *Handlers) StaffWorkspacesUnexempt(ctx context.Context, params staffapi.StaffWorkspacesUnexemptParams) (staffapi.StaffWorkspacesUnexemptRes, error) {
	w, changed, notFound, err := h.setExempt(ctx, params.WorkspaceId, false)
	if notFound {
		p := staffapi.StaffWorkspacesUnexemptNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &staffapi.StaffExemptionChange{Changed: changed, Workspace: staffMapper.WorkspaceToResource(w)}, nil
}

func (h *Handlers) setExempt(ctx context.Context, wsID staffapi.WorkspaceId, exempt bool) (w *ent.Workspace, changed, notFound bool, err error) {
	id := string(wsID)
	w, changed, err = h.module.SetAbuseExempt(ctx, h.bus, id, Current(ctx), exempt)
	if ent.IsNotFound(err) {
		return nil, false, true, nil
	}
	return w, changed, false, err
}
