package emailrender

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/osteele/liquid/expressions"
	"github.com/osteele/liquid/filters"
	"github.com/osteele/liquid/parser"
	"github.com/osteele/liquid/render"
	"github.com/osteele/liquid/values"
)

// The work one render may do (ADR 0174). A template is written by a Workspace Member or an API
// client and rendered in the process every Workspace shares, once per recipient, so what it may
// build and how long it may loop is bounded by the render, as Shopify's Liquid bounds it by a render
// score. The bounds are counts, not a clock, so a render ends the same way every time.
const (
	// MaxSteps is how many loop iterations, assigns and captures a render may run, whether the loop
	// body writes anything or not.
	MaxSteps = 5000
	// MaxValueBytes is the text one value may hold: an assign, a capture or a filter's result, a
	// string or the strings of an array together.
	MaxValueBytes = 256 << 10
	// MaxItems is how many items one array may hold: an assign, a filter's input or result, a range.
	MaxItems = 1000
	// MaxOutputBytes is how much a render may write.
	MaxOutputBytes = 1 << 20
)

// ErrTooMuchWork is a render that goes past a bound above: the content's own failure, the same on
// every attempt, so a send never retries it.
var ErrTooMuchWork = errors.New("the template does more work than a message may")

// budget is one render's count of steps, carried in its bindings under budgetKey.
type budget struct{ steps int }

// budgetKey is the binding the budget rides in: no template can name it, since a Liquid name never
// holds a NUL.
const budgetKey = "\x00budget"

// charge spends one step of the render's budget.
func charge(ctx render.Context) error {
	b, ok := ctx.Get(budgetKey).(*budget)
	if !ok {
		return nil
	}
	b.steps++
	if b.steps > MaxSteps {
		return fmt.Errorf("%w: more than %d loop iterations, assigns and captures", ErrTooMuchWork, MaxSteps)
	}
	return nil
}

// boundedEngine is a Liquid engine whose renders are bounded: osteele/liquid's own parser and renderer, with
// the loops, assign and capture counted and every filter's input and result measured.
type boundedEngine struct{ cfg render.Config }

// newEngine is an engine with no file tags that escapes each value it writes with escaper, or writes
// it as it is when escaper is nil.
func newEngine(escaper render.Replacer) *boundedEngine {
	cfg := render.NewConfig()
	cfg.TemplateStore = noFiles{}
	addTags(&cfg)
	guardFilters(&cfg)
	if escaper != nil {
		cfg.SetAutoEscapeReplacer(escaper)
	}
	return &boundedEngine{cfg: cfg}
}

// ErrUnparsable is a template the library panicked on rather than refused: a syntax it did not
// expect, such as an identifier of characters it cannot read.
var ErrUnparsable = errors.New("the template cannot be read")

// render parses source and renders it with bindings, within the bounds above. A panic of the library
// is the content's failure, never the process's.
func (e *boundedEngine) render(source string, bindings map[string]any) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = "", fmt.Errorf("%w: %v", ErrUnparsable, r)
		}
	}()
	if err := checkRanges(source); err != nil {
		return "", err
	}
	root, perr := e.cfg.Compile(source, parser.SourceLoc{})
	if perr != nil {
		return "", perr
	}
	vars := make(map[string]any, len(bindings)+1)
	maps.Copy(vars, bindings)
	vars[budgetKey] = &budget{}
	var b strings.Builder
	if rerr := render.Render(root, &cappedWriter{w: &b}, vars, e.cfg); rerr != nil {
		if overBudget(rerr) {
			return "", tooMuchWorkError{rerr}
		}
		return "", rerr
	}
	return b.String(), nil
}

// tooMuchWorkError is a render error a bound caused, kept as the library worded it (with the place
// in the template) and matching ErrTooMuchWork.
type tooMuchWorkError struct{ error }

func (t tooMuchWorkError) Unwrap() []error { return []error{t.error, ErrTooMuchWork} }

// overBudget reports whether err comes from a bound: the library carries a tag's error as its Cause
// and a filter's in a FilterError, neither of which unwraps.
func overBudget(err error) bool {
	for err != nil {
		if errors.Is(err, ErrTooMuchWork) {
			return true
		}
		if fe, ok := errors.AsType[expressions.FilterError](err); ok {
			err = fe.Err
			continue
		}
		c, ok := err.(interface{ Cause() error })
		if !ok || errors.Is(c.Cause(), err) {
			return false
		}
		err = c.Cause()
	}
	return false
}

// cappedWriter refuses a write past MaxOutputBytes.
type cappedWriter struct {
	w       io.Writer
	written int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.written+len(p) > MaxOutputBytes {
		return 0, fmt.Errorf("%w: more than %d bytes of output", ErrTooMuchWork, MaxOutputBytes)
	}
	n, err := c.w.Write(p)
	c.written += n
	return n, err
}

// rangePattern is a Liquid range, (from..to), in a tag or an output.
var rangePattern = regexp.MustCompile(`\(\s*([^()]*?)\s*\.\.\s*([^()]*?)\s*\)`)

// checkRanges refuses a range whose bounds are not whole numbers at most MaxItems apart. A range is
// the one value a template can make as large as it likes in a few characters, and a filter given
// one builds all of it, so its size is fixed where the template is written.
func checkRanges(source string) error {
	for _, tok := range parser.Scan(source, parser.SourceLoc{}, nil) {
		if tok.Type != parser.TagTokenType && tok.Type != parser.ObjTokenType {
			continue
		}
		for _, m := range rangePattern.FindAllStringSubmatch(tok.Args, -1) {
			from, errFrom := strconv.Atoi(m[1])
			to, errTo := strconv.Atoi(m[2])
			if errFrom != nil || errTo != nil {
				return fmt.Errorf("%w: a range takes two whole numbers, not (%s..%s)", ErrTooMuchWork, m[1], m[2])
			}
			if to-from+1 > MaxItems {
				return fmt.Errorf("%w: a range holds at most %d numbers", ErrTooMuchWork, MaxItems)
			}
		}
	}
	return nil
}

// assignTag is Liquid's assign, counted and with its value measured.
func assignTag(source string) (func(io.Writer, render.Context) error, error) {
	stmt, err := expressions.ParseStatement(expressions.AssignStatementSelector, source)
	if err != nil {
		return nil, err
	}
	if len(stmt.Path) > 1 {
		return nil, errors.New("syntax error: dot notation in assign tag is not supported")
	}
	return func(_ io.Writer, ctx render.Context) error {
		if err := charge(ctx); err != nil {
			return ctx.WrapError(err)
		}
		value, err := ctx.Evaluate(stmt.ValueFn)
		if err != nil {
			return err
		}
		if err := measured(value); err != nil {
			return ctx.WrapError(err)
		}
		ctx.Set(stmt.Assignment.Variable, value)
		return nil
	}, nil
}

// captureTag is Liquid's capture, counted and with its text measured.
func captureTag(node render.BlockNode) (func(io.Writer, render.Context) error, error) {
	name := strings.TrimSpace(node.Args)
	if name == "" || strings.ContainsAny(name, " \t") {
		return nil, fmt.Errorf("syntax error: capture tag requires exactly one variable name, got %q", node.Args)
	}
	return func(_ io.Writer, ctx render.Context) error {
		if err := charge(ctx); err != nil {
			return ctx.WrapError(err)
		}
		s, err := ctx.InnerString()
		if err != nil {
			return err
		}
		if len(s) > MaxValueBytes {
			return ctx.WrapError(fmt.Errorf("%w: a capture holds at most %d bytes", ErrTooMuchWork, MaxValueBytes))
		}
		ctx.Set(name, s)
		return nil
	}, nil
}

// filterSet records the standard filters as osteele/liquid defines them, so each can be registered
// measured instead.
type filterSet map[string]any

func (f filterSet) AddFilter(name string, fn any) { f[name] = fn }

var errorType = reflect.TypeFor[error]()

// guardFilters registers every standard filter so that its input, its arguments and its result are
// measured: none may be larger than one value may be. A filter keeps its parameters, so its
// arguments are converted as before, and returns its result with an error.
func guardFilters(cfg *render.Config) {
	std := filterSet{}
	filters.AddStandardFilters(std)
	for name, fn := range std {
		cfg.AddFilter(name, guarded(name, reflect.ValueOf(fn)))
	}
}

func guarded(name string, fn reflect.Value) any {
	t := fn.Type()
	in := make([]reflect.Type, t.NumIn())
	for i := range in {
		in[i] = t.In(i)
	}
	sig := reflect.FuncOf(in, []reflect.Type{reflect.TypeFor[any](), errorType}, t.IsVariadic())
	return reflect.MakeFunc(sig, func(args []reflect.Value) []reflect.Value {
		fail := func(err error) []reflect.Value {
			return []reflect.Value{reflect.Zero(reflect.TypeFor[any]()), reflect.ValueOf(fmt.Errorf("%s: %w", name, err))}
		}
		for _, a := range args {
			if a.Kind() == reflect.Func {
				continue
			}
			if err := measured(a.Interface()); err != nil {
				return fail(err)
			}
		}
		if check, ok := multiplying[name]; ok {
			if err := check(args); err != nil {
				return fail(err)
			}
		}
		var out []reflect.Value
		if t.IsVariadic() {
			out = fn.CallSlice(args)
		} else {
			out = fn.Call(args)
		}
		var result any
		for _, o := range out {
			if o.Type().Implements(errorType) {
				if err, ok := reflect.TypeAssert[error](o); ok && err != nil {
					return fail(err)
				}
				continue
			}
			result = o.Interface()
		}
		if err := measured(result); err != nil {
			return fail(err)
		}
		return []reflect.Value{reflect.ValueOf(&result).Elem(), reflect.Zero(errorType)}
	}).Interface()
}

// multiplying are the filters whose result can be many times their input: they are measured before
// they build it, not after.
var multiplying = map[string]func(args []reflect.Value) error{
	// replace puts the replacement in once per match, and an empty pattern matches between every
	// two characters.
	"replace": func(args []reflect.Value) error {
		s, old, repl := args[0].String(), args[1].String(), args[2].String()
		matches := strings.Count(s, old)
		return within(len(s) + matches*(len(repl)-len(old)))
	},
	// join puts the separator between every two items.
	"join": func(args []reflect.Value) error {
		items := args[0].Len()
		if items < 2 || args[1].Kind() != reflect.Func {
			return nil
		}
		sep := args[1].Call([]reflect.Value{reflect.ValueOf(" ")})[0].String()
		return within((items - 1) * len(sep))
	},
}

// within refuses a result of n bytes larger than one value may be.
func within(n int) error {
	if n > MaxValueBytes {
		return fmt.Errorf("%w: a value holds at most %d bytes", ErrTooMuchWork, MaxValueBytes)
	}
	return nil
}

// measured refuses a value larger than one value may be.
func measured(v any) error {
	bytes, items := 0, 0
	if !measure(reflect.ValueOf(v), &bytes, &items) {
		return fmt.Errorf("%w: a value holds at most %d bytes and %d items", ErrTooMuchWork, MaxValueBytes, MaxItems)
	}
	return nil
}

// measure adds the text and the items of v to bytes and items, and reports false as soon as either
// is past its bound.
func measure(v reflect.Value, bytes, items *int) bool {
	if !v.IsValid() {
		return true
	}
	if r, ok := reflect.TypeAssert[values.Range](v); ok {
		*items += r.Len()
		return *items <= MaxItems
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return true
		}
		return measure(v.Elem(), bytes, items)
	case reflect.String:
		*bytes += v.Len()
		return *bytes <= MaxValueBytes
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			*bytes += v.Len()
			return *bytes <= MaxValueBytes
		}
		*items += v.Len()
		if *items > MaxItems {
			return false
		}
		for i := range v.Len() {
			if !measure(v.Index(i), bytes, items) {
				return false
			}
		}
	case reflect.Map:
		*items += v.Len()
		if *items > MaxItems {
			return false
		}
		iter := v.MapRange()
		for iter.Next() {
			if !measure(iter.Key(), bytes, items) || !measure(iter.Value(), bytes, items) {
				return false
			}
		}
	}
	return true
}
