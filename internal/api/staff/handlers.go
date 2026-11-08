// Package staff is the core /staff surface (ADR 0026, ADR 0063): the Staff member's sign-in, the
// signed-in Staff member and the Instance's own settings (its Sign-up items and its Telemetry
// report), served on every Instance in both Modes and without a License key.
// The Enterprise sections of the console are a contract of their own (gen/staffee, ee/api/staff),
// served under the same prefix: Server falls through to them for every path this surface does
// not route.
package staff

import (
	"context"
	"fmt"
	"net/http"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/problem"
	"github.com/getsphericon/sphericon/internal/api/staff/resources"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
)

var (
	mapper = &resources.ConverterImpl{}
	inputs = &resources.InputsImpl{}
)

// Surface is the core /staff API: the handlers and the security handler over the Staff member
// module, for the composition root to mount.
type Surface struct {
	handlers *Handlers
	security *SecurityHandler
}

// NewSurface builds the surface over the Staff member module, the Sign-up items and the Instance
// state with its Telemetry report, or nil while the Staff sign-in is off (no Staff member secret),
// in which case nothing under /staff is served.
func NewSurface(m *staffmembers.Module, items *signupitems.Module, inst *instancestates.Module) *Surface {
	if m.Sessions() == nil {
		return nil
	}
	return &Surface{
		handlers: &Handlers{module: m, sessions: m.Sessions(), items: items, instance: inst},
		security: &SecurityHandler{sessions: m.Sessions()},
	}
}

// Server builds the ogen server under the /staff prefix. opts carry what the composition root
// owns (the RFC 7807 error handler, and the not-found handler that serves the Enterprise
// sections, or answers 404 without them).
func (s *Surface) Server(opts ...staffapi.ServerOption) (http.Handler, error) {
	return staffapi.NewServer(s.handlers, s.security,
		append([]staffapi.ServerOption{staffapi.WithPathPrefix(paths.Staff)}, opts...)...)
}

// Handlers implements staffapi.Handler.
type Handlers struct {
	module   *staffmembers.Module
	sessions *staffmembers.Sessions
	items    *signupitems.Module
	instance *instancestates.Module
}

var _ staffapi.Handler = (*Handlers)(nil)

// NewError answers a refused or failed request with the contract's Problem: the status and
// the body are decided once for every surface (internal/api/problem).
func (*Handlers) NewError(ctx context.Context, err error) *staffapi.ProblemDetailsStatusCode {
	return problem.Staff(ctx, err)
}

// StaffAuthLogin is the password step, run by the login module: it answers a challenge
// (or, at first login, an enrolment) and never a session. An unknown email and a wrong
// password answer the same 401.
func (h *Handlers) StaffAuthLogin(ctx context.Context, req *staffapi.StaffLoginInput) (*staffapi.StaffLoginResult, error) {
	res, err := h.module.CheckPassword(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.Throttled:
		return nil, throttled(ctx, res.Delay)
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
	case login.Authenticated:
		// The Staff login always has a second step.
	}
	return nil, fmt.Errorf("staff: unhandled login outcome %d", res.Outcome)
}

// StaffAuthSecondFactor is the second step: a valid challenge and a current TOTP
// code start the session. A bad challenge and a wrong code answer the same 401.
func (h *Handlers) StaffAuthSecondFactor(ctx context.Context, req *staffapi.StaffSecondFactorInput) (*staffapi.StaffResourceHeaders, error) {
	res, err := h.module.Complete(ctx, req.Challenge, req.Code)
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.SecondThrottled:
		return nil, throttled(ctx, res.Delay)
	case login.SecondAuthenticated:
		// No audit entry: the Staff member belongs to no Workspace.
		cookie, err := h.sessions.Issue(res.Principal)
		if err != nil {
			return nil, err
		}
		return &staffapi.StaffResourceHeaders{SetCookie: cookie.String(), Response: mapper.StaffMemberToResource(res.Principal)}, nil
	}
	return nil, fmt.Errorf("staff: unhandled login outcome %d", res.Outcome)
}

// StaffAuthLogout ends the Staff member's sessions server-side (the session epoch is
// bumped, so a token copied elsewhere stops verifying too) and clears the cookie.
func (h *Handlers) StaffAuthLogout(ctx context.Context) (*staffapi.StaffAuthLogoutNoContent, error) {
	if err := h.module.EndSessions(ctx, auth.StaffMember(ctx).ID); err != nil {
		return nil, err
	}
	return &staffapi.StaffAuthLogoutNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// StaffMeGet is the current Staff member.
func (h *Handlers) StaffMeGet(ctx context.Context) (*staffapi.StaffResource, error) {
	res := mapper.StaffMemberToResource(auth.StaffMember(ctx))
	return &res, nil
}

// throttled is the login throttle's refusal as the standard 429 (ADR 0025): the
// Retry-After and X-RateLimit headers are set on the response and the error handler
// renders the problem.
func throttled(ctx context.Context, res login.Delay) error {
	return ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, res.Limit, res.Wait, res.Now)
}

// SecurityHandler implements staffapi.SecurityHandler: the Staff member cookie.
type SecurityHandler struct{ sessions *staffmembers.Sessions }

var _ staffapi.SecurityHandler = (*SecurityHandler)(nil)

// HandleApiKeyAuth authenticates the Staff member cookie and puts the Staff member in the
// context.
func (h *SecurityHandler) HandleApiKeyAuth(ctx context.Context, _ staffapi.OperationName, t staffapi.ApiKeyAuth) (context.Context, error) {
	return auth.AuthenticateStaff(ctx, h.sessions, t.APIKey)
}
