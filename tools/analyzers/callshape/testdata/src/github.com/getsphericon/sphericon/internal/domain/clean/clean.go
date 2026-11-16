// Package clean is a domain module that keeps the call contract (ADR 0167).
package clean

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

// Bus does I/O; the constructor takes it.
type Bus interface {
	Publish(ctx context.Context, topic string) error
}

type Module struct {
	bus Bus
	now func() time.Time
}

// New takes every dependency and the clock.
func New(bus Bus, now func() time.Time) *Module { return &Module{bus: bus, now: now} }

// Actor is a value, an argument like any other.
type Actor struct{ ID int64 }

func (*Module) Record(context.Context, *ent.Scoped, Actor, string) (*Result, error) { return nil, nil }

func (*Module) Purge(context.Context, *ent.Client, int64) error { return nil }

// Rename takes the scope that stands for the Workspace lock and an entity: neither is a client.
func (*Module) Rename(context.Context, *ent.WorkspaceLocked, *ent.User, string) error { return nil }

// Since takes a time that is an input, not the clock.
func (*Module) Since(_ context.Context, _ *ent.Scoped, from time.Time) error { return nil }

// Detector is a role with dependencies of its own beside Module.
type Detector struct{ m *Module }

func NewDetector(m *Module) *Detector { return &Detector{m: m} }

func (*Detector) Detect(context.Context, *ent.Scoped) error { return nil }

type Result struct {
	ID    int64
	Codes []string
}

// Normalize is a pure helper: it takes no context.
func Normalize(s string) string { return s }

// Stamp reads the clock: the use case tier's own rule, which does not bind a domain module (another
// analyzer bans reading the wall clock everywhere).
func Stamp() time.Time { return time.Now() }

// store implements an interface whose signature it does not choose: a method of an unexported type
// is no operation of the module, whatever its shape.
type store struct{}

func (store) Lookup(context.Context, string, string, string) (int, bool, error) { return 0, false, nil }
