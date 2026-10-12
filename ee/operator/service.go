package operator

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/paths"
	"github.com/getsphericon/sphericon/internal/ratelimit"
)

// ServiceSurface is the service endpoints of the Operator surface (ADR 0026): what a
// service calls with a Service credential. It is a surface of its own, so the public
// Operator server has no route for any of it; the composition root mounts it only on
// the internal listener (ADR 0018).
type ServiceSurface struct {
	handlers *ServiceHandlers
	security *ServiceSecurityHandler
}

// NewServiceSurface builds the surface over the module that issues and checks credentials
// and the bus its writes are audited through.
func NewServiceSurface(m *Module, bus *events.Bus) *ServiceSurface {
	return &ServiceSurface{
		handlers: &ServiceHandlers{module: m, bus: bus},
		security: NewServiceSecurity(m, ServiceOperationScopes),
	}
}

// Server builds the ogen server under the /service prefix. opts carry what the
// composition root owns (the RFC 7807 error handler).
func (s *ServiceSurface) Server(opts ...serviceapi.ServerOption) (http.Handler, error) {
	return serviceapi.NewServer(s.handlers, s.security,
		append([]serviceapi.ServerOption{serviceapi.WithPathPrefix(paths.Service)}, opts...)...)
}

// ServiceOperationScopes names the scopes each operation of the service endpoints
// needs: a credential must carry all of them. An operation with no entry is refused to
// every credential, so a new endpoint cannot ship unguarded; an entry with no scopes is
// open to any valid credential. Add an endpoint's row with the endpoint.
var ServiceOperationScopes = map[serviceapi.OperationName][]Scope{
	serviceapi.ServiceIdentityGetOperation:       nil,
	serviceapi.ServiceWorkspaceFeedListOperation: {ScopeWorkspacesRead},
	serviceapi.ServiceHoldsListOperation:         {ScopeHoldsWrite},
	serviceapi.ServiceHoldsSetOperation:          {ScopeHoldsWrite},
	serviceapi.ServiceHoldsClearOperation:        {ScopeHoldsWrite},
}

// ServiceOperations lists every operation the service endpoints serve, from the
// generated handler contract.
func ServiceOperations() []serviceapi.OperationName {
	t := reflect.TypeFor[serviceapi.Handler]()
	names := make([]serviceapi.OperationName, 0, t.NumMethod())
	for i := range t.NumMethod() {
		names = append(names, t.Method(i).Name)
	}
	return names
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
	module   *Module
	requires map[serviceapi.OperationName][]Scope
}

var _ serviceapi.SecurityHandler = (*ServiceSecurityHandler)(nil)

// NewServiceSecurity builds the handler over a map of the scopes each operation needs.
func NewServiceSecurity(m *Module, requires map[serviceapi.OperationName][]Scope) *ServiceSecurityHandler {
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
	if errors.Is(err, ErrUnauthorized) {
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
		return ctx, ErrForbidden
	}
	if err := limits.ChargeService(cred.ID); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, serviceContextKey{}, cred), nil
}

func covers(have []string, need []Scope) bool {
	return !slices.ContainsFunc(need, func(s Scope) bool { return !slices.Contains(have, string(s)) })
}

// ServiceHandlers implements serviceapi.Handler.
type ServiceHandlers struct {
	module *Module
	bus    *events.Bus
}

var _ serviceapi.Handler = (*ServiceHandlers)(nil)

// ServiceIdentityGet is the calling credential's own name, scopes and expiry: the
// first endpoint on the internal listener, which proves the path end to end.
func (*ServiceHandlers) ServiceIdentityGet(ctx context.Context) (serviceapi.ServiceIdentityGetRes, error) {
	c := CurrentService(ctx)
	res := &serviceapi.ServiceIdentityResource{Name: c.Name, Scopes: make([]serviceapi.ServiceScope, len(c.Scopes))}
	for i, s := range c.Scopes {
		res.Scopes[i] = serviceapi.ServiceScope(s)
	}
	if c.ExpiresAt != nil {
		res.ExpiresAt = serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(*c.ExpiresAt))
	}
	return res, nil
}
