// Package sidelistener is an opt-in HTTP listener beside the public port (ADR 0018):
// the metrics listener and the internal listener of the Operator's service endpoints
// (ADR 0026) are both one. The network boundary is its control, so it binds only when
// an address is configured and a bind failure is fatal.
package sidelistener

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Server serves one handler on its own address. Listen binds the address up front so
// a bind failure is synchronous; Serve then accepts until Shutdown. A nil *Server
// means the listener is disabled: every method is a no-op on it.
type Server struct {
	name string
	srv  *http.Server
	ln   net.Listener
}

// New builds a server named name (for errors) that serves handler on addr (host:port).
func New(name, addr string, handler http.Handler) *Server {
	return &Server{name: name, srv: &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}}
}

// Listen binds the configured address.
func (s *Server) Listen(ctx context.Context) error {
	if s == nil {
		return nil
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("bind %s listener %s: %w", s.name, s.srv.Addr, err)
	}
	s.ln = ln
	return nil
}

// Addr is the bound address (after Listen), useful with an ephemeral port. It is
// empty before Listen.
func (s *Server) Addr() string {
	if s == nil || s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Serve serves on the bound listener and returns nil after a graceful Shutdown.
func (s *Server) Serve() error {
	if s == nil {
		return nil
	}
	if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
