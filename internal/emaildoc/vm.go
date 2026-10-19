package emaildoc

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/metrics"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// bundle is the editor's export code, built by `mise run build:emaildoc` from packages/emaildoc
// (the lockfile's @react-email/editor) and copied here. It is generated: regenerate, never edit.
//
//go:embed assets/emaildoc.js
var bundle string

// programOnce compiles the bundle once per Renderer (about 0.7 s); every runtime then runs the
// same read-only program.
type programOnce struct {
	once    sync.Once
	program *goja.Program
	err     error
}

func (p *programOnce) start() (*vm, error) {
	p.once.Do(func() {
		p.program, p.err = goja.Compile("emaildoc.js", bundle, false)
	})
	if p.err != nil {
		return nil, p.err
	}
	return newVM(p.program)
}

// vm is one goja runtime with the bundle loaded. goja runtimes are not safe for concurrent use: a
// vm serves one call at a time (the Renderer's slots see to that).
type vm struct {
	rt        *goja.Runtime
	check     func(goja.Value, ...goja.Value) (goja.Value, error)
	render    func(goja.Value, ...goja.Value) (goja.Value, error)
	runTimers func(goja.Value, ...goja.Value) (goja.Value, error)
}

func newVM(program *goja.Program) (*vm, error) {
	rt := goja.New()

	// The bundle's shims lean on these two for UTF-8 (a JavaScript version dominated the cost of
	// big documents) and on a silent console.
	must(rt.Set("goUtf8Encode", func(s string) goja.ArrayBuffer {
		return rt.NewArrayBuffer([]byte(s))
	}))
	must(rt.Set("goUtf8Decode", func(b goja.ArrayBuffer) string {
		return string(b.Bytes())
	}))
	must(rt.Set("console", map[string]any{
		"log": func() {}, "info": func() {}, "warn": func() {}, "error": func() {}, "debug": func() {},
	}))

	if _, err := rt.RunProgram(program); err != nil {
		return nil, fmt.Errorf("load the bundle: %w", err)
	}

	emaildoc := rt.Get("emaildoc")
	if emaildoc == nil {
		return nil, errors.New("the bundle did not define emaildoc")
	}
	object := emaildoc.ToObject(rt)
	v := &vm{rt: rt}
	for name, target := range map[string]*func(goja.Value, ...goja.Value) (goja.Value, error){
		"check":  &v.check,
		"render": &v.render,
	} {
		fn, ok := goja.AssertFunction(object.Get(name))
		if !ok {
			return nil, fmt.Errorf("the bundle's emaildoc.%s is not a function", name)
		}
		*target = fn
	}
	runTimers, ok := goja.AssertFunction(rt.Get("drainTimers"))
	if !ok {
		return nil, errors.New("the bundle did not define drainTimers")
	}
	v.runTimers = runTimers
	return v, nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// checkDoc asks the editor's schema whether the document fits.
func (v *vm) checkDoc(doc []byte) error {
	reason, err := v.check(goja.Undefined(), v.rt.ToValue(string(doc)))
	if err != nil {
		return fmt.Errorf("check the document: %w", err)
	}
	if goja.IsNull(reason) || goja.IsUndefined(reason) {
		return nil
	}
	return &InvalidError{Reason: "The document does not fit the editor's schema: " + reason.String()}
}

// renderDoc exports the document. The editor's export is async; the bundle's timer queue is
// drained here while its promise is pending.
func (v *vm) renderDoc(doc []byte) (Rendered, error) {
	value, err := v.render(goja.Undefined(), v.rt.ToValue(string(doc)))
	if err != nil {
		return Rendered{}, fmt.Errorf("render the document: %w", err)
	}
	promise, ok := value.Export().(*goja.Promise)
	if !ok {
		return Rendered{}, errors.New("render the document: the bundle did not return a promise")
	}
	for promise.State() == goja.PromiseStatePending {
		ran, err := v.runTimers(goja.Undefined())
		if err != nil {
			return Rendered{}, fmt.Errorf("render the document: %w", err)
		}
		if !ran.ToBoolean() {
			return Rendered{}, errors.New("render the document: the render never finished")
		}
	}
	if promise.State() == goja.PromiseStateRejected {
		return Rendered{}, &InvalidError{Reason: "The editor could not render the document: " + v.message(promise.Result())}
	}
	var out struct {
		HTML string `json:"html"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(promise.Result().String()), &out); err != nil {
		return Rendered{}, fmt.Errorf("read the render: %w", err)
	}
	return Rendered{HTML: out.HTML, Text: out.Text}, nil
}

func (v *vm) message(thrown goja.Value) string {
	if object, ok := thrown.(*goja.Object); ok {
		if message := object.Get("message"); message != nil {
			return message.String()
		}
	}
	return thrown.String()
}

// stopReason is why a guard stopped a call.
type stopReason int

const (
	stopNone stopReason = iota
	stopTimeout
	stopMemory
	stopCanceled
)

func (s stopReason) err(ctx context.Context) error {
	switch s {
	case stopTimeout:
		return ErrTimeout
	case stopMemory:
		return ErrMemoryLimit
	case stopCanceled:
		return ctx.Err()
	case stopNone:
	}
	return nil
}

const heapMetric = "/memory/classes/heap/objects:bytes"

func heapBytes() uint64 {
	sample := []metrics.Sample{{Name: heapMetric}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

// guard stops the call that is running on this vm when it runs past its time, when the process
// heap grows past the memory limit, or when ctx ends: it interrupts the runtime, which fails the
// call within milliseconds. The returned stop function ends the watch and says whether it had to
// interrupt; a runtime that was interrupted must be discarded.
func (v *vm) guard(ctx context.Context, cfg callConfig) (stop func() stopReason) {
	const memoryPoll = 20 * time.Millisecond
	done := make(chan struct{})
	result := make(chan stopReason, 1)
	baseline := heapBytes()

	go func() {
		timer := time.NewTimer(cfg.timeout)
		defer timer.Stop()
		poll := time.NewTicker(memoryPoll)
		defer poll.Stop()
		for {
			select {
			case <-done:
				result <- stopNone
				return
			case <-timer.C:
				v.rt.Interrupt("timeout")
				result <- stopTimeout
				return
			case <-ctx.Done():
				v.rt.Interrupt("canceled")
				result <- stopCanceled
				return
			case <-poll.C:
				if heapBytes() > baseline && heapBytes()-baseline > cfg.memoryGrowth {
					v.rt.Interrupt("memory")
					result <- stopMemory
					return
				}
			}
		}
	}()

	return func() stopReason {
		close(done)
		return <-result
	}
}
