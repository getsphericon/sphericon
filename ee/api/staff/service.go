package staff

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"

	eestaff "github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/api/problem"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
)

// ServiceSurface is the service endpoints of the Staff surface (ADR 0026): what a
// service calls with a Service credential. It is a surface of its own, so the public
// Staff member server has no route for any of it; the composition root mounts it only on
// the internal listener (ADR 0018).
type ServiceSurface struct {
	handlers *ServiceHandlers
	security *ServiceSecurityHandler
}

// NewServiceSurface builds the surface over the module that issues and checks credentials
// and audits its writes.
func NewServiceSurface(m *eestaff.Module) *ServiceSurface {
	return &ServiceSurface{
		handlers: &ServiceHandlers{module: m},
		security: NewServiceSecurity(m, ServiceOperationScopes),
	}
}

// Server builds the ogen server under the /service prefix. opts carry what the
// composition root owns (the RFC 7807 error handler).
func (s *ServiceSurface) Server(opts ...serviceapi.ServerOption) (http.Handler, error) {
	return serviceapi.NewServer(s.handlers, s.security,
		append([]serviceapi.ServerOption{serviceapi.WithPathPrefix(paths.Service)}, opts...)...)
}

// ServiceOperations lists every operation the service endpoints serve, from the
// generated handler contract. The contract's NewError, which answers the default Problem
// response of every operation (ADR 0103), is a method of that handler but no operation.
func ServiceOperations() []serviceapi.OperationName {
	t := reflect.TypeFor[serviceapi.Handler]()
	names := make([]serviceapi.OperationName, 0, t.NumMethod())
	for method := range t.Methods() {
		names = append(names, method.Name)
	}
	return slices.DeleteFunc(names, func(name string) bool { return name == "NewError" })
}

type serviceContextKey struct{}

// CurrentService is the Service credential an authenticated request belongs to.
func CurrentService(ctx context.Context) *ent.ServiceCredential {
	c, _ := ctx.Value(serviceContextKey{}).(*ent.ServiceCredential)
	return c
}

// ServiceSecurityHandler implements serviceapi.SecurityHandler: the bearer credential,
// its scopes and its rate limit.
type ServiceSecurityHandler struct {
	module   *eestaff.Module
	requires map[serviceapi.OperationName][]serviceapi.ServiceScope
}

var _ serviceapi.SecurityHandler = (*ServiceSecurityHandler)(nil)

// NewServiceSecurity builds the handler over a map of the scopes each operation needs.
func NewServiceSecurity(m *eestaff.Module, requires map[serviceapi.OperationName][]serviceapi.ServiceScope) *ServiceSecurityHandler {
	return &ServiceSecurityHandler{module: m, requires: requires}
}

// HandleBearerAuth authenticates the Service credential, refuses one whose scopes do
// not cover the operation (ErrForbidden), and applies the rate limits (ADR 0025): a
// client address that spent its failed-authentication budget is refused before the
// value is looked at, a failure counts against it, and a success charges the
// credential's own budget.
func (h *ServiceSecurityHandler) HandleBearerAuth(ctx context.Context, op serviceapi.OperationName, t serviceapi.BearerAuth) (context.Context, error) {
	limits := ratelimit.FromContext(ctx)
	if err := limits.AuthBlocked(ctx); err != nil {
		return ctx, err
	}
	cred, err := h.module.AuthenticateService(ctx, t.Token)
	if errors.Is(err, eestaff.ErrUnauthorized) {
		if limited := limits.AuthFailed(ctx); limited != nil {
			return ctx, limited
		}
		return ctx, err
	}
	if err != nil {
		return ctx, err
	}
	required, known := h.requires[op]
	if !known || !covers(cred.Scopes, required) {
		return ctx, eestaff.ErrForbidden
	}
	if err := limits.ChargeService(cred.ID); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, serviceContextKey{}, cred), nil
}

func covers(have []string, need []serviceapi.ServiceScope) bool {
	return !slices.ContainsFunc(need, func(s serviceapi.ServiceScope) bool { return !slices.Contains(have, string(s)) })
}

// ServiceHandlers implements serviceapi.Handler.
type ServiceHandlers struct {
	module *eestaff.Module
}

var _ serviceapi.Handler = (*ServiceHandlers)(nil)

// NewError answers a refused or failed request with the contract's Problem: the status and
// the body are decided once for every surface (internal/api/problem).
func (*ServiceHandlers) NewError(ctx context.Context, err error) *serviceapi.ProblemDetailsStatusCode {
	return problem.Service(ctx, err)
}

// ServiceIdentityGet is the calling credential's own name, scopes and expiry: the
// first endpoint on the internal listener, which proves the path end to end.
func (*ServiceHandlers) ServiceIdentityGet(ctx context.Context) (*serviceapi.ServiceIdentityResource, error) {
	res := serviceMapper.IdentityToResource(CurrentService(ctx))
	return &res, nil
}
