package operator

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/getsphericon/sphericon/internal/ratelimit"
)

// Surface is the /operator API: the handlers and the security handler over one
// Module, for the composition root to mount.
type Surface struct {
	handlers *Handlers
	security *SecurityHandler
}

// NewSurface builds the surface.
func NewSurface(m *Module, s *Sessions, bus *events.Bus, sender messaging.EmailSender) *Surface {
	return &Surface{handlers: &Handlers{module: m, sessions: s, bus: bus, sender: sender}, security: &SecurityHandler{sessions: s}}
}

// Server builds the ogen server under the /operator prefix. opts carry what the
// composition root owns (the RFC 7807 error handler).
func (s *Surface) Server(opts ...operatorapi.ServerOption) (http.Handler, error) {
	return operatorapi.NewServer(s.handlers, s.security,
		append([]operatorapi.ServerOption{operatorapi.WithPathPrefix(paths.Operator)}, opts...)...)
}

// Handlers implements operatorapi.Handler.
type Handlers struct {
	module   *Module
	sessions *Sessions
	bus      *events.Bus
	sender   messaging.EmailSender // the system (platform) sender that tells a suspended Workspace's owner
}

var _ operatorapi.Handler = (*Handlers)(nil)

// OperatorAuthLogin is the password step: it answers a challenge (or, at first login,
// an enrolment) and never a session. An unknown email and a wrong password answer the
// same 401.
func (h *Handlers) OperatorAuthLogin(ctx context.Context, req *operatorapi.OperatorLoginInput) (operatorapi.OperatorAuthLoginRes, error) {
	op, err := h.module.CheckPassword(ctx, req.Email, req.Password)
	if limited := throttled(ctx, err); limited != nil {
		return nil, limited
	}
	if errors.Is(err, ErrInvalidCredentials) {
		return unauthorized(i18n.T("errors.invalid_credentials", nil)), nil
	}
	if err != nil {
		return nil, err
	}
	step, err := h.module.StartSecondStep(ctx, op)
	if err != nil {
		return nil, err
	}
	res := &operatorapi.OperatorLoginResult{Outcome: operatorapi.OperatorLoginOutcomeChallenge, Challenge: step.Challenge}
	if step.Enrolment != nil {
		res.Outcome = operatorapi.OperatorLoginOutcomeEnrolment
		res.Enrolment = operatorapi.NewOptOperatorEnrolment(operatorapi.OperatorEnrolment{
			Secret: step.Enrolment.Secret, URI: step.Enrolment.URI, QrCode: step.Enrolment.QRCode,
		})
	}
	return res, nil
}

// OperatorAuthSecondFactor is the second step: a valid challenge and a current TOTP
// code start the session. A bad challenge and a wrong code answer the same 401.
func (h *Handlers) OperatorAuthSecondFactor(ctx context.Context, req *operatorapi.OperatorSecondFactorInput) (operatorapi.OperatorAuthSecondFactorRes, error) {
	op, err := h.module.Complete(ctx, req.Challenge, req.Code)
	if limited := throttled(ctx, err); limited != nil {
		return nil, limited
	}
	if errors.Is(err, ErrInvalidChallenge) {
		return unauthorized(i18n.T("errors.login_challenge_invalid", nil)), nil
	}
	if errors.Is(err, ErrInvalidCode) {
		return unauthorized(i18n.T("errors.second_factor_code_invalid", nil)), nil
	}
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(op)
	if err != nil {
		return nil, err
	}
	return &operatorapi.OperatorResourceHeaders{SetCookie: cookie.String(), Response: resource(op)}, nil
}

// OperatorAuthLogout ends the Operator's sessions server-side (the session epoch is
// bumped, so a token copied elsewhere stops verifying too) and clears the cookie.
func (h *Handlers) OperatorAuthLogout(ctx context.Context) (operatorapi.OperatorAuthLogoutRes, error) {
	if err := h.module.EndSessions(ctx, Current(ctx).ID); err != nil {
		return nil, err
	}
	return &operatorapi.OperatorAuthLogoutNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// OperatorMeGet is the current Operator.
func (h *Handlers) OperatorMeGet(ctx context.Context) (operatorapi.OperatorMeGetRes, error) {
	res := resource(Current(ctx))
	return &res, nil
}

// throttled turns the login throttle's refusal into the standard 429 (ADR 0025): the
// Retry-After and X-RateLimit headers are set on the response and the error handler
// renders the problem. It is nil for any other error and for nil.
func throttled(ctx context.Context, err error) error {
	var t *ThrottledError
	if !errors.As(err, &t) {
		return nil
	}
	return ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, t.Limit, t.Wait, t.Now)
}

func resource(op *ent.Operator) operatorapi.OperatorResource {
	return operatorapi.OperatorResource{ID: operatorapi.EntityId(strconv.FormatInt(op.ID, 10)), Email: operatorapi.EmailAddress(op.Email)}
}

// problem is the RFC 7807 body of a refusal.
type problem = operatorapi.ProblemDetails

// unauthorized is the 401 of a failed login step. It satisfies both login responses.
func unauthorized(detail string) *problem {
	return &problem{
		Status: operatorapi.NewOptInt32(http.StatusUnauthorized),
		Title:  operatorapi.NewOptString(http.StatusText(http.StatusUnauthorized)),
		Detail: operatorapi.NewOptString(detail),
	}
}

// OperatorWorkspacesList is a page of Workspaces, newest first, narrowed by a slug
// search. Metadata only: no Contacts, content or Events.
func (h *Handlers) OperatorWorkspacesList(ctx context.Context, params operatorapi.OperatorWorkspacesListParams) (operatorapi.OperatorWorkspacesListRes, error) {
	page, err := h.module.ListWorkspaces(ctx, review(params.Review), strings.TrimSpace(params.Slug.Value), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]operatorapi.OperatorWorkspaceResource, 0, len(page.Items))
	for _, w := range page.Items {
		items = append(items, workspaceResource(w))
	}
	return &operatorapi.OperatorWorkspacesListOK{
		Items:      items,
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func review(p operatorapi.OptOperatorWorkspaceReview) Review {
	switch p.Value {
	case operatorapi.OperatorWorkspaceReviewWarned:
		return ReviewWarned
	case operatorapi.OperatorWorkspaceReviewSystemFrozen:
		return ReviewSystemFrozen
	default:
		return ReviewNone
	}
}

// OperatorWorkspacesGet is one Workspace's metadata and suspension state.
func (h *Handlers) OperatorWorkspacesGet(ctx context.Context, params operatorapi.OperatorWorkspacesGetParams) (operatorapi.OperatorWorkspacesGetRes, error) {
	id, ok := workspaceID(params.WorkspaceId)
	if !ok {
		return workspaceNotFound(), nil
	}
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
	active, err := h.module.ActiveHolds(ctx, id)
	if err != nil {
		return nil, err
	}
	breach, err := h.module.WorkspaceAbuse(ctx, w)
	if err != nil {
		return nil, err
	}
	res := workspaceDetail(w, d, active, breach)
	return &res, nil
}

// workspaceDetail is the detail page's view: the metadata of workspaceResource plus
// the deliverability counts and the active holds.
func workspaceDetail(w *ent.Workspace, d Deliverability, active []*ent.Hold, warning abuse.Breach) operatorapi.OperatorWorkspaceDetailResource {
	shown := make([]operatorapi.OperatorHold, len(active))
	for i, r := range active {
		shown[i] = operatorapi.OperatorHold{
			Key: r.Key, ReasonCode: r.ReasonCode, Message: r.Message, SetBy: r.SetBy,
			CreatedAt: operatorapi.Timestamp(r.CreatedAt), ExpiresAt: operatorapi.Timestamp(r.ExpiresAt), Billing: holds.IsBilling(r.ReasonCode),
		}
	}
	base := workspaceResource(w)
	domains := make([]operatorapi.OperatorDomainRates, 0, len(d.Domains))
	for _, r := range d.Domains {
		domains = append(domains, operatorapi.OperatorDomainRates{
			SendingDomainId: operatorapi.EntityId(strconv.FormatInt(r.SendingDomainID, 10)),
			Domain:          r.Domain,
			ComplaintRate:   rateResource(r.Complaint),
			BounceRate:      rateResource(r.Bounce),
		})
	}
	return operatorapi.OperatorWorkspaceDetailResource{
		ID: base.ID, Slug: base.Slug, Name: base.Name, CreatedAt: base.CreatedAt, Suspension: base.Suspension, Exempt: base.Exempt, Warned: base.Warned, Holds: shown,
		Abuse: abuseResource(w, warning),
		Deliverability: operatorapi.OperatorDeliverability{
			WindowHours: int32(d.Window / time.Hour),
			VolumeFloor: int32(d.VolumeFloor),
			SendVolume:  int32(d.SendVolume),
			Domains:     domains,
		},
	}
}

func abuseResource(w *ent.Workspace, warning abuse.Breach) operatorapi.OperatorAbuse {
	res := operatorapi.OperatorAbuse{
		Exempt:            w.AbuseExempt,
		ClearedAt:         optNilTimestamp(w.ClearedAt),
		WarningNotifiedAt: optNilTimestamp(w.AbuseWarningNotifiedAt),
		FreezeNotifiedAt:  optNilTimestamp(w.AbuseFreezeNotifiedAt),
	}
	if warning.Tier != abuse.TierNone {
		res.Warning = operatorapi.NewOptNilOperatorAbuseWarning(operatorapi.OperatorAbuseWarning{
			Domain: warning.Domain, Metric: operatorapi.OperatorAbuseMetric(warning.Metric), Rate: warning.Rate(),
		})
	}
	return res
}

func optNilTimestamp(t *time.Time) operatorapi.OptNilTimestamp {
	if t == nil {
		return operatorapi.OptNilTimestamp{}
	}
	return operatorapi.NewOptNilTimestamp(operatorapi.Timestamp(*t))
}

func rateResource(r Rate) operatorapi.OperatorRate {
	out := operatorapi.OperatorRate{Numerator: int32(r.Numerator), Denominator: int32(r.Denominator)}
	if r.Rate == nil {
		out.Rate = operatorapi.NilFloat64{Null: true}
	} else {
		out.Rate = operatorapi.NewNilFloat64(*r.Rate)
	}
	return out
}

// workspaceID parses the path's Workspace id; false is a malformed one, which the
// handlers answer as not found like an unknown id.
func workspaceID(id operatorapi.EntityId) (int64, bool) {
	n, err := strconv.ParseInt(string(id), 10, 64)
	return n, err == nil
}

func workspaceNotFound() *operatorapi.OperatorWorkspacesGetNotFound {
	p := operatorapi.OperatorWorkspacesGetNotFound(notFoundProblem())
	return &p
}

func notFoundProblem() operatorapi.ProblemDetails {
	return operatorapi.ProblemDetails{
		Status: operatorapi.NewOptInt32(http.StatusNotFound),
		Title:  operatorapi.NewOptString(http.StatusText(http.StatusNotFound)),
		Detail: operatorapi.NewOptString(i18n.T("errors.workspace_not_found", nil)),
	}
}

func change(w *ent.Workspace, changed bool) *operatorapi.OperatorSuspensionChange {
	return &operatorapi.OperatorSuspensionChange{Changed: changed, Workspace: workspaceResource(w)}
}

// OperatorWorkspacesSuspend freezes a Workspace's outbound sending as the signed-in
// Operator, whose id the audit trail keeps while customers see "sphericon staff".
func (h *Handlers) OperatorWorkspacesSuspend(ctx context.Context, req *operatorapi.OperatorSuspendInput, params operatorapi.OperatorWorkspacesSuspendParams) (operatorapi.OperatorWorkspacesSuspendRes, error) {
	id, ok := workspaceID(params.WorkspaceId)
	if !ok {
		p := operatorapi.OperatorWorkspacesSuspendNotFound(notFoundProblem())
		return &p, nil
	}
	w, changed, err := h.module.SuspendWorkspace(ctx, h.bus, h.sender, id, Current(ctx), req.Reason)
	if errors.Is(err, ErrReasonRequired) {
		p := operatorapi.OperatorWorkspacesSuspendUnprocessableEntity{
			Status: operatorapi.NewOptInt32(http.StatusUnprocessableEntity),
			Title:  operatorapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
			Detail: operatorapi.NewOptString(i18n.T("errors.suspension_reason_required", nil)),
		}
		return &p, nil
	}
	if ent.IsNotFound(err) {
		p := operatorapi.OperatorWorkspacesSuspendNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return change(w, changed), nil
}

// OperatorWorkspacesUnsuspend lifts a Workspace's suspension as the signed-in Operator.
func (h *Handlers) OperatorWorkspacesUnsuspend(ctx context.Context, params operatorapi.OperatorWorkspacesUnsuspendParams) (operatorapi.OperatorWorkspacesUnsuspendRes, error) {
	id, ok := workspaceID(params.WorkspaceId)
	if !ok {
		p := operatorapi.OperatorWorkspacesUnsuspendNotFound(notFoundProblem())
		return &p, nil
	}
	w, changed, err := h.module.UnsuspendWorkspace(ctx, h.bus, id, Current(ctx))
	if ent.IsNotFound(err) {
		p := operatorapi.OperatorWorkspacesUnsuspendNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return change(w, changed), nil
}

// OperatorWorkspacesExempt exempts a Workspace from the abuse detector as the signed-in
// Operator.
func (h *Handlers) OperatorWorkspacesExempt(ctx context.Context, params operatorapi.OperatorWorkspacesExemptParams) (operatorapi.OperatorWorkspacesExemptRes, error) {
	w, changed, notFound, err := h.setExempt(ctx, params.WorkspaceId, true)
	if notFound {
		p := operatorapi.OperatorWorkspacesExemptNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return exemptionChange(w, changed), nil
}

// OperatorWorkspacesUnexempt puts a Workspace back under the abuse detector.
func (h *Handlers) OperatorWorkspacesUnexempt(ctx context.Context, params operatorapi.OperatorWorkspacesUnexemptParams) (operatorapi.OperatorWorkspacesUnexemptRes, error) {
	w, changed, notFound, err := h.setExempt(ctx, params.WorkspaceId, false)
	if notFound {
		p := operatorapi.OperatorWorkspacesUnexemptNotFound(notFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return exemptionChange(w, changed), nil
}

func (h *Handlers) setExempt(ctx context.Context, wsID operatorapi.EntityId, exempt bool) (w *ent.Workspace, changed, notFound bool, err error) {
	id, ok := workspaceID(wsID)
	if !ok {
		return nil, false, true, nil
	}
	w, changed, err = h.module.SetAbuseExempt(ctx, h.bus, id, Current(ctx), exempt)
	if ent.IsNotFound(err) {
		return nil, false, true, nil
	}
	return w, changed, false, err
}

func exemptionChange(w *ent.Workspace, changed bool) *operatorapi.OperatorExemptionChange {
	return &operatorapi.OperatorExemptionChange{Changed: changed, Workspace: workspaceResource(w)}
}

// workspaceResource is the console's view of a Workspace: metadata and suspension
// state only. The Operator sees the real actor id, which a customer never does.
func workspaceResource(w *ent.Workspace) operatorapi.OperatorWorkspaceResource {
	res := operatorapi.OperatorWorkspaceResource{
		ID:        operatorapi.EntityId(strconv.FormatInt(w.ID, 10)),
		Slug:      w.Slug,
		Name:      w.Name,
		CreatedAt: operatorapi.Timestamp(w.CreatedAt),
		Exempt:    w.AbuseExempt,
		Warned:    w.AbuseWarningNotifiedAt != nil,
	}
	if w.SuspendedAt == nil {
		return res
	}
	actor := operatorapi.OperatorSuspensionActor{}
	if w.SuspendedByKind != nil {
		actor.Kind = operatorapi.OperatorSuspensionActorKind(*w.SuspendedByKind)
	}
	if w.SuspendedByKind != nil && *w.SuspendedByKind == workspace.SuspendedByKindOperator && w.SuspendedByID != nil {
		actor.ID = operatorapi.NewOptNilString(*w.SuspendedByID)
	}
	s := operatorapi.OperatorSuspension{At: operatorapi.Timestamp(*w.SuspendedAt), Actor: actor}
	if w.SuspensionReason != nil {
		s.Reason = operatorapi.NewOptNilString(*w.SuspensionReason)
	}
	res.Suspension = operatorapi.NewOptNilOperatorSuspension(s)
	return res
}
