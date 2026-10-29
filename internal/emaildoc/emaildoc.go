// Package emaildoc turns an editor document into email-ready HTML, with limits: a document in, HTML
// or an error out (ADR 0040).
//
// A document is the JSON of @react-email/editor. The render is the editor's own export code (a
// generated bundle, `mise run build:emaildoc`) run in an embedded JavaScript runtime (goja): no Node,
// no sidecar, no cgo. The first runtime takes seconds to start and tens of megabytes, and the
// languages a code block highlights take seconds more to register on a runtime, so a Renderer
// starts nothing itself: Warm starts it in the background (the server does, at boot) or the first
// call does, and a runtime registers the languages before its first code block; calls queue for
// it. A call that runs past its time or memory limit is
// stopped and its runtime is discarded (a stopped runtime does not recover), and a fresh one starts
// on the next call. Inputs are capped (size, depth, node count) before they reach a runtime.
//
// The server builds one Renderer and hands it to every module that saves a document. A test that
// does not test the export passes an Engine of its own (WithEngine), so only the packages that do
// test it pay for a runtime.
//
// A document is also validated: structurally against the editor's schema and by rules of ours
// (heading levels, a button's attributes, a link's href, allowed attribute values). An invalid
// document is refused with a reason an agent can act on; nothing is repaired or dropped.
package emaildoc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
	// StartTimeout is how long a call waits for a runtime to start (the first call of a process,
	// or the first after one was discarded) and, before the first code block a runtime renders,
	// for the code block's languages to register, on top of Timeout. Each takes seconds, many more
	// on a busy machine; a call that gives up leaves the work running for the next one.
	StartTimeout time.Duration
	// MemoryGrowth is how many bytes the process heap may grow during one call. The heap counter is
	// process-wide, so another goroutine's allocations count too; the call is stopped only when the
	// growth stays over the limit for a fifth of a second, which a passing burst does not, so the
	// limit is a guard, not an accounting.
	MemoryGrowth uint64
}

// DefaultLimits are sized for an email: the prototype rendered 1,000 paragraphs in about 8 seconds
// and 1,000 nested levels in about 10, so a document stays well under both.
func DefaultLimits() Limits {
	return Limits{
		MaxBytes:     256 << 10,
		MaxDepth:     24,
		MaxNodes:     3000,
		Timeout:      30 * time.Second * raceSlowdown,
		StartTimeout: 2 * time.Minute * raceSlowdown,
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
	// languages is set once a runtime has registered the code block's languages before a call.
	languages atomic.Bool
	// engine, when set, stands in for the runtime: the editor's schema check and export run there,
	// after our own checks, and no runtime is ever started.
	engine Engine
}

// Engine is the editor's own code, the part of a render that runs after our checks: its schema
// check and its export. The Renderer runs the real one in goja; WithEngine passes another.
type Engine interface {
	// Check answers nil when the document fits the editor's schema, else an *InvalidError.
	Check(doc []byte) error
	// Export renders a document that passed Check.
	Export(doc []byte) (Rendered, error)
}

// Option configures New.
type Option func(*Renderer, *int)

// WithRuntimes sets how many runtimes the Renderer may run at once (default 1). Each costs about 16
// MB and seconds to start, the first about 68 MB.
func WithRuntimes(n int) Option { return func(_ *Renderer, runtimes *int) { *runtimes = max(n, 1) } }

// WithEngine runs engine in place of the editor's code in goja: a test that saves documents
// without testing their export passes a fake, and starts no runtime.
func WithEngine(engine Engine) Option { return func(r *Renderer, _ *int) { r.engine = engine } }

// New returns a Renderer. It starts nothing: the first call does, or Warm.
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

// Started reports whether a runtime has been started yet.
func (r *Renderer) Started() bool { return r.started.Load() }

// LanguagesRegistered reports whether a runtime has registered the languages a code block
// highlights yet, which it does before the first code block it renders (outside the render's time
// limit) or at Warm, and never for a document without one.
func (r *Renderer) LanguagesRegistered() bool { return r.languages.Load() }

// Warm starts the runtime in the background, with the code block's languages, and returns at once,
// so a server that calls it at boot has a runtime ready by the time the first document is saved.
// Without it that first call waits for the start, which takes longer than a request may run on a
// small machine, and gives up with ErrRenderUnavailable. A start that fails leaves the runtime to
// the first call, as before. How long the start took is logged, since it varies with the machine.
// The start itself cannot be interrupted, so ctx ending only stops waiting for it.
func (r *Renderer) Warm(ctx context.Context) {
	if r.engine != nil {
		return
	}
	go func() {
		began := time.Now()
		err := r.withRuntime(ctx, r.config(), true, func(*vm) error { return nil })
		if err != nil {
			slog.Warn("document runtime warm-up failed", "error", err, "duration_ms", time.Since(began).Milliseconds())
			return
		}
		slog.Info("document runtime ready", "duration_ms", time.Since(began).Milliseconds())
	}()
}

type callConfig struct {
	timeout      time.Duration
	memoryGrowth uint64
}

func (r *Renderer) config() callConfig {
	return callConfig{timeout: r.limits.Timeout, memoryGrowth: r.limits.MemoryGrowth}
}

// Validate answers nil when doc is a document the editor can hold and Render can export, else an
// error that says why: an *InvalidError, or a limit error.
func (r *Renderer) Validate(ctx context.Context, doc []byte) error {
	if _, err := r.prepare(doc); err != nil {
		return err
	}
	if r.engine != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return r.engine.Check(doc)
	}
	return r.withRuntime(ctx, r.config(), false, func(rt *vm) error {
		return rt.checkDoc(doc)
	})
}

// Render validates doc and exports it as an email: HTML and a plain-text part. It never repairs a
// document; an invalid one is refused with the reason.
func (r *Renderer) Render(ctx context.Context, doc []byte) (Rendered, error) {
	root, err := r.prepare(doc)
	if err != nil {
		return Rendered{}, err
	}
	if r.engine != nil {
		if err := ctx.Err(); err != nil {
			return Rendered{}, err
		}
		if err := r.engine.Check(doc); err != nil {
			return Rendered{}, err
		}
		return r.engine.Export(doc)
	}
	var out Rendered
	err = r.withRuntime(ctx, r.config(), holdsCodeBlock(root), func(rt *vm) error {
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

// prepare caps the document and checks it against our rules, before any runtime is involved, and
// returns its parsed node tree.
func (r *Renderer) prepare(doc []byte) (any, error) {
	root, err := parseWithin(doc, r.limits)
	if err != nil {
		return nil, err
	}
	return root, checkRules(root)
}

// holdsCodeBlock says whether a node tree that passed prepare holds a code block, whose export
// needs the code block's languages.
func holdsCodeBlock(node any) bool {
	object, ok := node.(map[string]any)
	if !ok {
		return false
	}
	if object["type"] == "codeBlock" {
		return true
	}
	children, _ := object["content"].([]any)
	return slices.ContainsFunc(children, holdsCodeBlock)
}

// startResult is a runtime start that finished.
type startResult struct {
	rt  *vm
	err error
}

// readyWithin readies the runtime of the slot the caller holds: it starts one when the slot has
// none (rt is nil) and registers the code block's languages when the call needs them and the
// runtime has not yet. It waits no longer than ctx and the Renderer's StartTimeout (this work has a
// bound of its own, apart from the call's time limit). The work itself cannot be interrupted, so a
// call that gives up leaves it running: the slot is handed back when it finishes, with the runtime
// when it is ready, so the work is not lost and the next call finds the slot busy only for as long
// as it takes. On error the caller no longer holds the slot.
func (r *Renderer) readyWithin(ctx context.Context, rt *vm, languages bool) (*vm, error) {
	finished := make(chan startResult, 1)
	go func() {
		rt, err := r.ready(rt, languages)
		finished <- startResult{rt: rt, err: err}
	}()
	timer := time.NewTimer(r.limits.StartTimeout)
	defer timer.Stop()

	var abandoned error
	select {
	case res := <-finished:
		if res.err != nil {
			r.slots <- nil
			return nil, fmt.Errorf("ready the document runtime: %w", res.err)
		}
		r.started.Store(true)
		return res.rt, nil
	case <-ctx.Done():
		abandoned = ctx.Err()
	case <-timer.C:
		abandoned = ErrTimeout
	}
	go func() {
		res := <-finished
		if res.err == nil {
			r.started.Store(true)
		}
		r.slots <- res.rt
	}()
	return nil, abandoned
}

// ready starts a runtime when rt is nil and registers the code block's languages on it when
// languages asks for them. A runtime whose registration failed is discarded.
func (r *Renderer) ready(rt *vm, languages bool) (*vm, error) {
	if rt == nil {
		started, err := r.program.start()
		if err != nil {
			return nil, err
		}
		rt = started
	}
	if languages && !rt.languages {
		if err := rt.loadLanguages(); err != nil {
			return nil, err
		}
		r.languages.Store(true)
	}
	return rt, nil
}

// withRuntime runs fn on a runtime under the call's limits, after the code block's languages are
// registered on it when languages asks for them. A runtime that was stopped, or whose state is
// unknown after an error, is discarded.
func (r *Renderer) withRuntime(ctx context.Context, cfg callConfig, languages bool, fn func(*vm) error) error {
	// A call whose context has ended takes no slot, even when one is free.
	if err := ctx.Err(); err != nil {
		return err
	}
	var rt *vm
	select {
	case rt = <-r.slots:
	case <-ctx.Done():
		return ctx.Err()
	}
	if rt == nil || (languages && !rt.languages) {
		ready, err := r.readyWithin(ctx, rt, languages)
		if err != nil {
			return err
		}
		rt = ready
	}
	defer func() { r.slots <- rt }()

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
