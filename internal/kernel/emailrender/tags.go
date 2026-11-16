package emailrender

// The standard tags of Liquid, adapted from osteele/liquid v1.9.2 (tags/iteration_tags.go,
// tags/control_flow_tags.go, tags/standard_tags.go) so that each loop iteration, assign and capture
// spends a step of the render's budget (ADR 0174): the library's loop has no hook, a loop whose body
// writes nothing would run unseen, and a block the library defines cannot be defined again, so every
// tag is registered here. Only the charges, the measuring of assign and capture (budget.go) and the
// refused include and render differ from the library.
//
// Copyright (c) 2017 Oliver Steele. Used under the MIT License:
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
// associated documentation files (the "Software"), to deal in the Software without restriction,
// including without limitation the rights to use, copy, modify, merge, publish, distribute,
// sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions: The above copyright notice and this
// permission notice shall be included in all copies or substantial portions of the Software. THE
// SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT
// LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT
// OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"

	"github.com/osteele/liquid/expressions"
	"github.com/osteele/liquid/render"
	"github.com/osteele/liquid/values"
)

const forloopVarName = "forloop"

var (
	errLoopContinue = errors.New("continue outside a loop")
	errLoopBreak    = errors.New("break outside a loop")
)

// addTags registers the standard tags. include and render read no file: template authors are
// Workspace members and API clients, never trusted with the server's files.
func addTags(cfg *render.Config) {
	refused := func(string) (func(io.Writer, render.Context) error, error) {
		return func(_ io.Writer, ctx render.Context) error { return ctx.WrapError(errFileTags) }, nil
	}
	cfg.AddTag("assign", assignTag)
	cfg.AddTag("include", refused)
	cfg.AddTag("render", refused)
	cfg.AddTag("break", func(string) (func(io.Writer, render.Context) error, error) {
		return func(_ io.Writer, ctx render.Context) error { return ctx.WrapError(errLoopBreak) }, nil
	})
	cfg.AddTag("continue", func(string) (func(io.Writer, render.Context) error, error) {
		return func(_ io.Writer, ctx render.Context) error { return ctx.WrapError(errLoopContinue) }, nil
	})
	cfg.AddTag("cycle", cycleTag)
	cfg.AddBlock("capture").Compiler(captureTag)
	cfg.AddBlock("case").Clause("when").Clause("else").Compiler(caseTag)
	cfg.AddBlock("comment")
	cfg.AddBlock("for").Clause("else").Compiler(loopTag)
	cfg.AddBlock("if").Clause("else").Clause("elsif").Compiler(ifTag(true))
	cfg.AddBlock("raw")
	cfg.AddBlock("tablerow").Compiler(loopTag)
	cfg.AddBlock("unless").Clause("else").Compiler(ifTag(false))
}

func cycleTag(args string) (func(io.Writer, render.Context) error, error) {
	stmt, err := expressions.ParseStatement(expressions.CycleStatementSelector, args)
	if err != nil {
		return nil, err
	}
	cycle := stmt.Cycle
	return func(w io.Writer, ctx render.Context) error {
		loopRec, ok := ctx.Get(forloopVarName).(map[string]any)
		if !ok {
			return ctx.Errorf("cycle must be within a forloop")
		}
		cycles, ok := loopRec[".cycles"].(map[string]int)
		if !ok {
			return ctx.Errorf("cycle must be within a forloop")
		}
		n := cycles[cycle.Group]
		cycles[cycle.Group] = n + 1
		_, err := io.WriteString(w, cycle.Values[n%len(cycle.Values)])
		return err
	}, nil
}

func caseTag(node render.BlockNode) (func(io.Writer, render.Context) error, error) {
	expr, err := expressions.Parse(node.Args)
	if err != nil {
		return nil, err
	}
	type caseClause struct {
		when []expressions.Expression // nil for else
		body *render.BlockNode
	}
	cases := make([]caseClause, 0, len(node.Clauses))
	for _, clause := range node.Clauses {
		if clause.Name != "when" {
			cases = append(cases, caseClause{body: clause})
			continue
		}
		stmt, err := expressions.ParseStatement(expressions.WhenStatementSelector, clause.Args)
		if err != nil {
			return nil, err
		}
		cases = append(cases, caseClause{when: stmt.Exprs, body: clause})
	}
	return func(w io.Writer, ctx render.Context) error {
		sel, err := ctx.Evaluate(expr)
		if err != nil {
			return err
		}
		for _, c := range cases {
			if c.when == nil {
				return ctx.RenderBlock(w, c.body)
			}
			for _, e := range c.when {
				v, err := ctx.Evaluate(e)
				if err != nil {
					return err
				}
				if values.Equal(sel, v) {
					return ctx.RenderBlock(w, c.body)
				}
			}
		}
		return nil
	}, nil
}

func ifTag(polarity bool) func(render.BlockNode) (func(io.Writer, render.Context) error, error) {
	return func(node render.BlockNode) (func(io.Writer, render.Context) error, error) {
		type branch struct {
			test expressions.Expression
			body *render.BlockNode
		}
		expr, err := expressions.Parse(node.Args)
		if err != nil {
			return nil, err
		}
		if !polarity {
			expr = expressions.Not(expr)
		}
		branches := []branch{{expr, &node}}
		for _, c := range node.Clauses {
			test := expressions.Constant(true)
			if c.Name == "elsif" {
				if test, err = expressions.Parse(c.Args); err != nil {
					return nil, err
				}
			}
			branches = append(branches, branch{test, c})
		}
		return func(w io.Writer, ctx render.Context) error {
			for _, b := range branches {
				value, err := ctx.Evaluate(b.test)
				if err != nil {
					return err
				}
				if value != nil && value != false {
					return ctx.RenderBlock(w, b.body)
				}
			}
			return nil
		}, nil
	}
}

type iterable interface {
	Len() int
	Index(i int) any
}

func loopTag(node render.BlockNode) (func(io.Writer, render.Context) error, error) {
	stmt, err := expressions.ParseStatement(expressions.LoopStatementSelector, node.Args)
	if err != nil {
		return nil, err
	}
	return func(w io.Writer, ctx render.Context) error {
		val, err := ctx.Evaluate(stmt.Expr)
		if err != nil {
			return err
		}
		iter := makeIterator(val)
		if iter == nil {
			return nil
		}
		iter, err = applyLoopModifiers(stmt.Loop, ctx, iter)
		if err != nil {
			return err
		}
		if len(node.Clauses) > 1 {
			return errors.New("for loops accept at most one else clause")
		}
		if iter.Len() == 0 && len(node.Clauses) == 1 && node.Clauses[0].Name == "else" {
			return ctx.RenderBlock(w, node.Clauses[0])
		}
		return loopRenderer{stmt.Loop, node.Name}.render(iter, w, ctx)
	}, nil
}

type loopRenderer struct {
	expressions.Loop

	tagName string
}

func (loop loopRenderer) render(iter iterable, w io.Writer, ctx render.Context) error {
	decorator, err := makeLoopDecorator(loop, ctx)
	if err != nil {
		return err
	}
	defer func(index, forloop any) {
		ctx.Set(forloopVarName, index)
		ctx.Set(loop.Variable, forloop)
	}(ctx.Get(forloopVarName), ctx.Get(loop.Variable))

	cycleMap := map[string]int{}
	forloopMap := map[string]any{
		"first": false, "last": false, "index": 0, "index0": 0, "rindex": 0, "rindex0": 0, "length": 0,
		".cycles": cycleMap,
	}
	ctx.Set(forloopVarName, forloopMap)

loop:
	for i, l := 0, iter.Len(); i < l; i++ {
		if err := charge(ctx); err != nil {
			return ctx.WrapError(err)
		}
		ctx.Set(loop.Variable, iter.Index(i))
		forloopMap["first"] = i == 0
		forloopMap["last"] = i == l-1
		forloopMap["index"] = i + 1
		forloopMap["index0"] = i
		forloopMap["rindex"] = l - i
		forloopMap["rindex0"] = l - i - 1
		forloopMap["length"] = l
		if err := decorator.before(w, i); err != nil {
			return err
		}
		rerr := ctx.RenderChildren(w)
		if err := decorator.after(w, i, l); err != nil {
			return err
		}
		switch {
		case rerr == nil:
		case errors.Is(rerr.Cause(), errLoopBreak):
			break loop
		case errors.Is(rerr.Cause(), errLoopContinue):
			continue loop
		default:
			return rerr
		}
	}
	return nil
}

func makeLoopDecorator(loop loopRenderer, ctx render.Context) (loopDecorator, error) {
	if loop.tagName != "tablerow" {
		return forLoopDecorator{}, nil
	}
	if loop.Cols != nil {
		val, err := ctx.Evaluate(loop.Cols)
		if err != nil {
			return nil, err
		}
		cols, ok := val.(int)
		if !ok {
			return nil, ctx.Errorf("loop cols must be an integer")
		}
		if cols > 0 {
			return tableRowDecorator(cols), nil
		}
	}
	return tableRowDecorator(math.MaxInt32), nil
}

type loopDecorator interface {
	before(w io.Writer, i int) error
	after(w io.Writer, i, l int) error
}

type forLoopDecorator struct{}

func (forLoopDecorator) before(io.Writer, int) error     { return nil }
func (forLoopDecorator) after(io.Writer, int, int) error { return nil }

type tableRowDecorator int

func (c tableRowDecorator) before(w io.Writer, i int) error {
	cols := int(c)
	row, col := i/cols, i%cols
	if col == 0 {
		if _, err := fmt.Fprintf(w, `<tr class="row%d">`, row+1); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, `<td class="col%d">`, col+1)
	return err
}

func (c tableRowDecorator) after(w io.Writer, i, l int) error {
	cols := int(c)
	if _, err := io.WriteString(w, `</td>`); err != nil {
		return err
	}
	if (i+1)%cols == 0 || i+1 == l {
		_, err := io.WriteString(w, `</tr>`)
		return err
	}
	return nil
}

func applyLoopModifiers(loop expressions.Loop, ctx render.Context, iter iterable) (iterable, error) {
	if loop.Reversed {
		iter = reverseWrapper{iter}
	}
	if loop.Offset != nil {
		val, err := ctx.Evaluate(loop.Offset)
		if err != nil {
			return nil, err
		}
		offset, ok := val.(int)
		if !ok {
			return nil, ctx.Errorf("loop offset must be an integer")
		}
		if offset > 0 {
			iter = offsetWrapper{iter, offset}
		}
	}
	if loop.Limit != nil {
		val, err := ctx.Evaluate(loop.Limit)
		if err != nil {
			return nil, err
		}
		limit, ok := val.(int)
		if !ok {
			return nil, ctx.Errorf("loop limit must be an integer")
		}
		if limit >= 0 {
			iter = limitWrapper{iter, limit}
		}
	}
	return iter, nil
}

func makeIterator(value any) iterable {
	if iter, ok := value.(iterable); ok {
		return iter
	}
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return keysOf(m)
	}
	switch reflect.TypeOf(value).Kind() {
	case reflect.Array, reflect.Slice:
		return sliceWrapper(reflect.ValueOf(value))
	case reflect.Map:
		rv := reflect.ValueOf(value)
		array := make([][]any, rv.Len())
		for i, k := range rv.MapKeys() {
			array[i] = []any{k.Interface(), rv.MapIndex(k).Interface()}
		}
		return sliceWrapper(reflect.ValueOf(array))
	default:
		return nil
	}
}

// keysOf iterates a map of the bindings as the library iterates one: by its pairs, in key order.
func keysOf(m map[string]any) iterable {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([][]any, len(keys))
	for i, k := range keys {
		pairs[i] = []any{k, m[k]}
	}
	return sliceWrapper(reflect.ValueOf(pairs))
}

type sliceWrapper reflect.Value

func (w sliceWrapper) Len() int        { return reflect.Value(w).Len() }
func (w sliceWrapper) Index(i int) any { return reflect.Value(w).Index(i).Interface() }

type limitWrapper struct {
	i iterable
	n int
}

func (w limitWrapper) Len() int        { return min(w.n, w.i.Len()) }
func (w limitWrapper) Index(i int) any { return w.i.Index(i) }

type offsetWrapper struct {
	i iterable
	n int
}

func (w offsetWrapper) Len() int        { return max(0, w.i.Len()-w.n) }
func (w offsetWrapper) Index(i int) any { return w.i.Index(i + w.n) }

type reverseWrapper struct{ i iterable }

func (w reverseWrapper) Len() int        { return w.i.Len() }
func (w reverseWrapper) Index(i int) any { return w.i.Index(w.i.Len() - 1 - i) }
