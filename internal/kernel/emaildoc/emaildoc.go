// Package emaildoc turns an editor document into email-ready HTML, with limits: a document in, HTML
// or an error out (ADR 0040).
//
// A document is the JSON of @react-email/editor. The render is the editor's own export code (a
// generated bundle, `mise run build:emaildoc`) run in an embedded JavaScript runtime (goja): no Node,
// no sidecar, no cgo. A runtime takes tens of megabytes and up to a second to start on a quiet
// machine, many more on a busy one, and the languages a code block highlights take seconds more
// to register on it, so a Renderer
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

	"github.com/jackc/puddle/v2"

	"github.com/getsphericon/sphericon/internal/kernel/narrow"
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
	// StartTimeout is how long a call waits for a runtime, on top of Timeout: its turn when every
	// runtime is busy, the start of one (the first call of a process, or the first after one was
	// discarded) and, before the first code block a runtime renders, the code block's languages to
	// register. A start and a registration take seconds, many more on a busy machine; a call that
	// gives up leaves the work running for the next one.
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
	// HTML is the full email document, unformatted: no whitespace the editor's markup does not
	// hold itself, so a whole email can be one line.
	HTML string
	// Text is the plain-text part derived from it.
	Text string
}

// Renderer renders and validates documents. It is safe for concurrent use. The zero value is not
// usable: build one with New.
type Renderer struct {
	limits Limits
	// runtimes holds the runtimes a Renderer may have, each started by the first call that finds
	// none idle while there is room for another. A call acquires one, so calls queue when every
	// runtime is busy.
	runtimes *puddle.Pool[*vm]

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
	pool, err := puddle.NewPool(&puddle.Config[*vm]{
		// The start cannot be interrupted, so the constructor ignores its context: puddle keeps
		// the start running for a caller that gave up on it and hands the runtime to the next.
		Constructor: func(context.Context) (*vm, error) { return startVM() },
		// A runtime holds nothing but memory, which the collector takes back.
		Destructor: func(*vm) {},
		MaxSize:    narrow.Int32(runtimes),
	})
	must(err) // only a MaxSize under 1, which WithRuntimes rules out
	r.runtimes = pool
	return r
}

// Started reports whether a runtime is ready: started, and not discarded since.
func (r *Renderer) Started() bool {
	stat := r.runtimes.Stat()
	return stat.TotalResources() > stat.ConstructingResources()
}

// LanguagesRegistered reports whether a runtime has registered the languages a code block
// highlights yet, which it does before the first code block it renders (outside the render's time
// limit) or at Warm, and never for a document without one.
func (r *Renderer) LanguagesRegistered() bool { return r.languages.Load() }

// Warm starts the runtime in the background, with the code block's languages, and returns at once,
// so a server that calls it at boot has a runtime ready by the time the first document is saved.
// Without it that first call waits for the start, which takes longer than a request may run on a
// small machine, and gives up with ErrRenderUnavailable. A start that fails leaves the runtime to
// the first call, as before. How long the start took is logged, since it varies with the machine.
// The start itself cannot be interrupted, so ctx ending only stops waiting for it. The channel it
// returns is closed once the warm-up is over, ready or not, for a caller that waits on it.
func (r *Renderer) Warm(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if r.engine != nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		began := time.Now()
		err := r.withRuntime(ctx, r.config(), true, func(*vm) error { return nil })
		if err != nil {
			slog.WarnContext(ctx, "document runtime warm-up failed", "error", err, "duration_ms", time.Since(began).Milliseconds())
			return
		}
		slog.InfoContext(ctx, "document runtime ready", "duration_ms", time.Since(began).Milliseconds())
	}()
	return done
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
	if object["type"] == nodeCodeBlock {
		return true
	}
	children, _ := object["content"].([]any)
	return slices.ContainsFunc(children, holdsCodeBlock)
}

// withRuntime runs fn on a runtime under the call's limits, after the code block's languages are
// registered on it when languages asks for them. Readying the runtime (waiting for one, starting
// one, registering the languages) waits no longer than ctx and the Renderer's StartTimeout. A
// runtime that was stopped, or whose state is unknown after an error, is discarded.
func (r *Renderer) withRuntime(ctx context.Context, cfg callConfig, languages bool, fn func(*vm) error) error {
	readyCtx, cancel := context.WithTimeout(ctx, r.limits.StartTimeout)
	res, err := r.acquire(readyCtx, languages)
	// Read before cancel, which would end readyCtx whatever became of the call.
	timedOut := errors.Is(readyCtx.Err(), context.DeadlineExceeded)
	cancel()
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return ctx.Err()
	case timedOut:
		return ErrTimeout
	default:
		return fmt.Errorf("ready the document runtime: %w", err)
	}

	// The runtime goes back to the pool only when the call ended cleanly; a panic discards it too.
	keep := false
	defer func() { settle(res, keep) }()
	rt := res.Value()
	stop := rt.guard(ctx, cfg)
	err = fn(rt)
	reason := stop()
	switch {
	case reason != stopNone:
		return reason.err(ctx)
	case err != nil && !errors.Is(err, ErrInvalid):
		return err
	}
	keep = true
	return err
}

// settle hands a runtime back to the pool when keep says it is fit for the next call, and
// discards it otherwise.
func settle(res *puddle.Resource[*vm], keep bool) {
	if keep {
		res.Release()
		return
	}
	res.Destroy()
}

// acquire takes a runtime from the pool, which starts one when none is idle and the Renderer may
// have another, and registers the code block's languages on it when languages asks for them and
// the runtime has not yet. A call whose ctx has ended takes no runtime, even an idle one. Neither
// the start nor the registration can be interrupted, so a call that gives up leaves them running:
// puddle hands a started runtime to the next call, and the registration below hands its runtime
// back once it finishes, so the work is not lost.
func (r *Renderer) acquire(ctx context.Context, languages bool) (*puddle.Resource[*vm], error) {
	res, err := r.runtimes.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	rt := res.Value()
	if !languages || rt.languages {
		return res, nil
	}
	registered := make(chan error, 1)
	go func() { registered <- rt.loadLanguages() }()
	select {
	case err := <-registered:
		if err := r.settleLanguages(res, err); err != nil {
			return nil, err
		}
		return res, nil
	case <-ctx.Done():
		go func() {
			err := r.settleLanguages(res, <-registered)
			if err != nil {
				slog.WarnContext(ctx, "document runtime discarded after a registration nobody waited for", "error", err)
				return
			}
			res.Release()
		}()
		return nil, ctx.Err()
	}
}

// settleLanguages records a registration of the code block's languages that ended: a runtime whose
// registration failed is discarded, and the error returned.
func (r *Renderer) settleLanguages(res *puddle.Resource[*vm], err error) error {
	if err != nil {
		res.Destroy()
		return err
	}
	r.languages.Store(true)
	return nil
}
