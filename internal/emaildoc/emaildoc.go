// Package emaildoc turns an editor document into email-ready HTML, with limits: a document in, HTML
// or an error out (ADR 0040).
//
// A document is the JSON of @react-email/editor. The render is the editor's own export code (a
// generated bundle, `mise run build:emaildoc`) run in an embedded JavaScript runtime (goja): no Node,
// no sidecar, no cgo. A runtime takes seconds to start and tens of megabytes, so it starts on the
// first call, not at boot; calls queue for it. A call that runs past its time or memory limit is
// stopped and its runtime is discarded (a stopped runtime does not recover), and a fresh one starts
// on the next call. Inputs are capped (size, depth, node count) before they reach a runtime.
//
// A document is also validated: structurally against the editor's schema and by rules of ours
// (heading levels, a button's attributes, a link's href, allowed attribute values). An invalid
// document is refused with a reason an agent can act on; nothing is repaired or dropped.
package emaildoc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Errors a call can end with, to be told apart with errors.Is. An invalid document is an
// *InvalidError (errors.Is(err, ErrInvalid)).
var (
	ErrInvalid      = errors.New("the document is invalid")
	ErrTooLarge     = errors.New("the document is too large")
	ErrTooDeep      = errors.New("the document is nested too deeply")
	ErrTooManyNodes = errors.New("the document has too many nodes")
	ErrTimeout      = errors.New("the render took too long and was stopped")
	ErrMemoryLimit  = errors.New("the render used too much memory and was stopped")
)

// InvalidError is a document that the editor cannot hold, with the reason in words an author can
// act on.
type InvalidError struct {
	Reason string
}

func (e *InvalidError) Error() string { return e.Reason }

// Is makes every *InvalidError match ErrInvalid.
func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

// Limits cap a document before it reaches a runtime.
type Limits struct {
	// MaxBytes is the size of the document's JSON.
	MaxBytes int
	// MaxDepth is how deep nodes nest (the document is 1).
	MaxDepth int
	// MaxNodes is how many nodes the document holds in all, text nodes included.
	MaxNodes int
	// Timeout is how long one call may run once a runtime is ready.
	Timeout time.Duration
	// MemoryGrowth is how many bytes the process heap may grow during one call. The heap counter is
	// process-wide, so another goroutine's allocations count too; the limit is a guard, not an
	// accounting.
	MemoryGrowth uint64
}

// DefaultLimits are sized for an email: the prototype rendered 1,000 paragraphs in about 8 seconds
// and 1,000 nested levels in about 10, so a document stays well under both.
func DefaultLimits() Limits {
	return Limits{
		MaxBytes:     256 << 10,
		MaxDepth:     24,
		MaxNodes:     3000,
		Timeout:      30 * time.Second,
		MemoryGrowth: 384 << 20,
	}
}

// Rendered is the editor's export of a document.
type Rendered struct {
	// HTML is the full email document, formatted.
	HTML string
	// Text is the plain-text part derived from it.
	Text string
}

// Renderer renders and validates documents. It is safe for concurrent use. The zero value is not
// usable: build one with New.
type Renderer struct {
	limits Limits
	// slots holds one entry per runtime a Renderer may have; nil is a runtime not started (or one
	// discarded). A call takes a slot, so calls queue when every runtime is busy.
	slots chan *vm

	program programOnce
	started atomic.Bool
}

// Option configures New.
type Option func(*Renderer, *int)

// WithLimits replaces DefaultLimits.
func WithLimits(l Limits) Option { return func(r *Renderer, _ *int) { r.limits = l } }

// WithRuntimes sets how many runtimes the Renderer may run at once (default 1). Each costs about 16
// MB and seconds to start, the first about 68 MB.
func WithRuntimes(n int) Option { return func(_ *Renderer, runtimes *int) { *runtimes = max(n, 1) } }

// New returns a Renderer. It starts nothing: the first call does.
func New(opts ...Option) *Renderer {
	r := &Renderer{limits: DefaultLimits()}
	runtimes := 1
	for _, opt := range opts {
		opt(r, &runtimes)
	}
	r.slots = make(chan *vm, runtimes)
	for range runtimes {
		r.slots <- nil
	}
	return r
}

var (
	defaultOnce     sync.Once
	defaultRenderer *Renderer
)

// Default is the process-wide Renderer with the default limits and one runtime.
func Default() *Renderer {
	defaultOnce.Do(func() { defaultRenderer = New() })
	return defaultRenderer
}

// Started reports whether a runtime has been started yet.
func (r *Renderer) Started() bool { return r.started.Load() }

// CallOption configures one Render or Validate call.
type CallOption func(*callConfig)

type callConfig struct {
	timeout      time.Duration
	memoryGrowth uint64
}

// WithTimeout shortens this call's time limit (it never lengthens the Renderer's).
func WithTimeout(d time.Duration) CallOption {
	return func(c *callConfig) { c.timeout = min(c.timeout, d) }
}

// WithMemoryGrowth shortens this call's memory limit (it never lengthens the Renderer's).
func WithMemoryGrowth(bytes uint64) CallOption {
	return func(c *callConfig) { c.memoryGrowth = min(c.memoryGrowth, bytes) }
}

func (r *Renderer) config(opts []CallOption) callConfig {
	c := callConfig{timeout: r.limits.Timeout, memoryGrowth: r.limits.MemoryGrowth}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// Validate answers nil when doc is a document the editor can hold and Render can export, else an
// error that says why: an *InvalidError, or a limit error.
func (r *Renderer) Validate(ctx context.Context, doc []byte, opts ...CallOption) error {
	if err := r.prepare(doc); err != nil {
		return err
	}
	return r.withRuntime(ctx, r.config(opts), func(rt *vm) error {
		return rt.checkDoc(doc)
	})
}

// Render validates doc and exports it as an email: HTML and a plain-text part. It never repairs a
// document; an invalid one is refused with the reason.
func (r *Renderer) Render(ctx context.Context, doc []byte, opts ...CallOption) (Rendered, error) {
	if err := r.prepare(doc); err != nil {
		return Rendered{}, err
	}
	var out Rendered
	err := r.withRuntime(ctx, r.config(opts), func(rt *vm) error {
		if err := rt.checkDoc(doc); err != nil {
			return err
		}
		var err error
		out, err = rt.renderDoc(doc)
		return err
	})
	if err != nil {
		return Rendered{}, err
	}
	return out, nil
}

// prepare caps the document and checks it against our rules, before any runtime is involved.
func (r *Renderer) prepare(doc []byte) error {
	root, err := parseWithin(doc, r.limits)
	if err != nil {
		return err
	}
	return checkRules(root)
}

// withRuntime runs fn on a runtime under the call's limits. A runtime that was stopped, or whose
// state is unknown after an error, is discarded.
func (r *Renderer) withRuntime(ctx context.Context, cfg callConfig, fn func(*vm) error) error {
	var rt *vm
	select {
	case rt = <-r.slots:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { r.slots <- rt }()

	if rt == nil {
		started, err := r.program.start()
		if err != nil {
			return fmt.Errorf("start the document runtime: %w", err)
		}
		rt = started
		r.started.Store(true)
	}

	stop := rt.guard(ctx, cfg)
	err := fn(rt)
	reason := stop()
	switch {
	case reason != stopNone:
		rt = nil
		return reason.err(ctx)
	case err != nil && !errors.Is(err, ErrInvalid):
		rt = nil
		return err
	}
	return err
}
