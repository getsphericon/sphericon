// Package bad is a domain module that breaks the call contract (ADR 0167).
package bad

import (
	"context"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
)

func Operation(context.Context) error { return nil } // want `rule 5: Operation is an exported function that takes a context`

// Helper takes no context: a pure helper.
func Helper(n int) int { return n }

func unexported(context.Context) error { return nil }

type Module struct{}

func New() *Module { return &Module{} }

func (*Module) Tuple(context.Context, *ent.Scoped) (int, []string, error) { return 0, nil, nil } // want `rule 5: Tuple returns 3 values; .*\(ADR 0167\)`

func (*Module) Pair(context.Context, *ent.Scoped) (int, string) { return 0, "" } // want `rule 5: Pair returns a second value that is not error`

func (*Module) Named(context.Context, *ent.Scoped) (verified bool, err error) { return false, nil } // want `rule 5: Named has named results`

func (*Module) Each(context.Context, *ent.Scoped, []string) []error { return nil } // want `rule 5: Each returns \[\]error`

func (*Module) Three(context.Context, *ent.Scoped, int64, string, string) error { return nil } // want `rule 5: Three takes 3 arguments after ctx and the scope`

func (*Module) At(_ context.Context, _ *ent.Scoped, now time.Time) error { return nil } // want `rule 5: At takes now`

func (*Module) Clocked(context.Context, func() time.Time) error { return nil } // want `rule 5: Clocked takes a clock`

// Split takes no context: a pure method is no operation, whatever its shape.
func (*Module) Split(string) (string, string, bool) { return "", "", false }

func (*Module) unexported(context.Context) (int, int, error) { return 0, 0, nil }

// Detector is a role of its own beside Module; its constructor may fail, which the contract refuses.
type Detector struct{}

func NewDetector() (*Detector, error) { return &Detector{}, nil } // want `rule 5: NewDetector returns more than \*Detector`

func (*Detector) Run(context.Context) error { return nil }

type Orphan struct{} // want `rule 5: Orphan has operations but no constructor New or NewOrphan that returns \*Orphan`

func (*Orphan) Do(context.Context) error { return nil }

// Summary is a value type: its methods take no context, so it needs no constructor.
type Summary struct{ N int }

func (s Summary) Total() int { return s.N }

// Bus does I/O: a method of it takes a context.
type Bus struct{}

func NewBus() *Bus { return &Bus{} }

func (*Bus) Publish(context.Context, string) error { return nil }

// Resolver does I/O behind an interface.
type Resolver interface {
	Lookup(ctx context.Context, host string) ([]string, error)
}

// Actor is a value: its methods take no context.
type Actor struct{ ID int64 }

func (a Actor) String() string { return "" }

func (*Module) Publishing(context.Context, *ent.Scoped, *Bus) error { return nil } // want `rule 5: Publishing takes a client \(\*bad.Bus\)`

func (*Module) Resolving(context.Context, Resolver) error { return nil } // want `rule 5: Resolving takes a client \(bad.Resolver\)`

func (*Module) Fetching(context.Context, *http.Client) error { return nil } // want `rule 5: Fetching takes a client \(\*http.Client\)`

func (*Module) Raw(context.Context, *ent.Scoped, *ent.Client) error { return nil } // want `rule 5: Raw takes a client \(\*ent.Client\)`

func (*Module) Acting(context.Context, *ent.Scoped, Actor) error { return nil }

// Mailer takes its dependencies through functional options.
type Mailer struct{ bus *Bus }

type MailerOption func(*Mailer)

func WithBus(b *Bus) MailerOption { return func(m *Mailer) { m.bus = b } }

func NewMailer(opts ...MailerOption) *Mailer { return &Mailer{} } // want `rule 5: NewMailer takes functional options`

func (*Mailer) Send(context.Context, *ent.Scoped, string) error { return nil }

// NewIDs is a constructor of a value; a variadic argument that is no function is no option.
func NewIDs(ids ...int64) []int64 { return ids }

func (*Module) RawScope(context.Context, *ent.Client, Actor) error { return nil }
