// Package driverops checks that a driver function calls one operation of the core (ADR 0111,
// review rule 5).
//
// A driver is the code that receives input from outside the program, and an operation of the core
// is a call into a domain module or a use case that takes a context.Context: which packages those
// are is the statement of the tiers (tools/analyzers/tier). Reading the actor or the scope the middleware put in the context and mapping to
// and from the contract take no such call, so they are no operation.
//
// The count follows the function's control flow: a driver that calls one operation on each branch
// of a switch calls one per run, while two calls on one path, or one in a loop, compose an
// operation the driver should not hold. A function of the driver package that the driver calls
// costs its own operations at the call site, so a helper hides none. A function literal is a
// driver function of its own (a command's action, a consumer), counted apart from the function
// that builds it.
package driverops

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/cfg"
	"golang.org/x/tools/go/types/typeutil"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// Analyzer reports a driver function that calls more than one operation of the core.
var Analyzer = &analysis.Analyzer{
	Name:     "driverops",
	Doc:      "refuses a driver function that calls more than one operation of the core (ADR 0111)",
	Run:      run,
	Requires: []*analysis.Analyzer{ctrlflow.Analyzer},
}

const fix = "move the composition into the domain module that owns its rule, or into a use case when it spans several, so the driver makes one call (ADR 0111)"

// cost is what a path of a function calls: the most operations along one path, and whether an
// operation runs in a loop.
type cost struct {
	ops  []string
	loop bool
}

func (c cost) plus(d cost) cost {
	return cost{ops: append(slices.Clip(c.ops), d.ops...), loop: c.loop || d.loop}
}

// max is the costlier path; a loop over an operation outweighs any count.
func (c cost) max(d cost) cost {
	if c.loop != d.loop {
		if c.loop {
			return c
		}
		return d
	}
	if len(d.ops) > len(c.ops) {
		return d
	}
	return c
}

type checker struct {
	pass  *analysis.Pass
	cfgs  *ctrlflow.CFGs
	decls map[*types.Func]*ast.FuncDecl
	// costs memoizes a declared function's cost; one being costed holds a zero cost, so recursion ends.
	costs map[*ast.FuncDecl]cost
	// err is the first function ctrlflow built no graph for; the run fails with it.
	err error
}

func run(pass *analysis.Pass) (any, error) {
	if !tier.IsDriver(pass.Pkg.Path()) {
		return nil, nil
	}
	// Without the control-flow graphs no function could be costed, and the package would pass
	// unread: a missing prerequisite fails the run instead.
	cfgs, ok := pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs)
	if !ok {
		return nil, fmt.Errorf("driverops: no result of %s for %s", ctrlflow.Analyzer.Name, pass.Pkg.Path())
	}
	c := &checker{
		pass:  pass,
		cfgs:  cfgs,
		decls: map[*types.Func]*ast.FuncDecl{},
		costs: map[*ast.FuncDecl]cost{},
	}
	var files []*ast.File
	for _, file := range pass.Files {
		if analysisx.InTestFile(pass, file.Pos()) {
			continue
		}
		files = append(files, file)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				if obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func); ok {
					c.decls[obj] = fn
				}
			}
		}
	}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				if n.Body != nil {
					c.report(n.Pos(), n.Name.Name, c.declCost(n))
				}
			case *ast.FuncLit:
				c.report(n.Pos(), "a function literal", c.graphCost(n.Pos(), c.cfgs.FuncLit(n)))
			}
			return true
		})
	}
	return nil, c.err
}

func (c *checker) report(pos token.Pos, name string, k cost) {
	ops := strings.Join(slices.Compact(slices.Sorted(slices.Values(k.ops))), ", ")
	switch {
	case k.loop:
		rule.Report(c.pass, pos, 5, fmt.Sprintf("%s calls an operation of the core in a loop (%s)", name, ops), fix)
	case len(k.ops) > 1:
		rule.Report(c.pass, pos, 5, fmt.Sprintf("%s calls %d operations of the core (%s)", name, len(k.ops), ops), fix)
	}
}

// declCost is the cost of a declared function of the package, computed once.
func (c *checker) declCost(fn *ast.FuncDecl) cost {
	if k, ok := c.costs[fn]; ok {
		return k
	}
	c.costs[fn] = cost{}
	k := c.graphCost(fn.Pos(), c.cfgs.FuncDecl(fn))
	c.costs[fn] = k
	return k
}

// graphCost is the costliest path from the entry of g, the graph of the function at pos. Blocks
// are condensed into their strongly connected components: a component with a cycle is a loop, and
// an operation inside it runs once per turn. A function with a body always has a graph, so a
// missing one is an error of the run, never a function that costs nothing.
func (c *checker) graphCost(pos token.Pos, g *cfg.CFG) cost {
	if g == nil || len(g.Blocks) == 0 {
		if c.err == nil {
			c.err = fmt.Errorf("driverops: %s built no control-flow graph for the function at %s", ctrlflow.Analyzer.Name, c.pass.Fset.Position(pos))
		}
		return cost{}
	}
	comps, of := components(g.Blocks[0])
	// Tarjan's algorithm finds the components in reverse topological order, so every successor's
	// component is done before its predecessors'.
	best := make([]cost, len(comps))
	for i, comp := range comps {
		var own cost
		for _, b := range comp {
			own = own.plus(c.blockCost(b))
		}
		if len(own.ops) > 0 && cyclic(comp, of, i) {
			own.loop = true
		}
		var next cost
		for _, b := range comp {
			for _, s := range b.Succs {
				if j := of[s]; j != i {
					next = next.max(best[j])
				}
			}
		}
		best[i] = own.plus(next)
	}
	return best[len(comps)-1]
}

// blockCost is the cost of the statements and conditions of a block.
func (c *checker) blockCost(b *cfg.Block) cost {
	var k cost
	for _, n := range b.Nodes {
		ast.Inspect(n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CallExpr:
				k = k.plus(c.callCost(n))
			}
			return true
		})
	}
	return k
}

func (c *checker) callCost(call *ast.CallExpr) cost {
	// The function or method the call names, generic instances resolved to their origin.
	f, _ := typeutil.Callee(c.pass.TypesInfo, call).(*types.Func)
	if f == nil || f.Pkg() == nil {
		return cost{}
	}
	if tier.IsCore(f.Pkg().Path()) && analysisx.TakesContext(f.Signature()) {
		return cost{ops: []string{name(f)}}
	}
	if fn, ok := c.decls[f]; ok {
		return c.declCost(fn)
	}
	return cost{}
}

// components are the strongly connected components of the blocks reachable from entry, in
// reverse topological order (Tarjan), with the index of each block's component.
func components(entry *cfg.Block) (comps [][]*cfg.Block, of map[*cfg.Block]int) {
	of = map[*cfg.Block]int{}
	var (
		index = map[*cfg.Block]int{}
		low   = map[*cfg.Block]int{}
		on    = map[*cfg.Block]bool{}
		stack []*cfg.Block
		visit func(b *cfg.Block)
	)
	visit = func(b *cfg.Block) {
		index[b], low[b] = len(index), len(index)
		stack = append(stack, b)
		on[b] = true
		for _, s := range b.Succs {
			if _, seen := index[s]; !seen {
				visit(s)
				low[b] = min(low[b], low[s])
			} else if on[s] {
				low[b] = min(low[b], index[s])
			}
		}
		if low[b] != index[b] {
			return
		}
		var comp []*cfg.Block
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			on[top] = false
			of[top] = len(comps)
			comp = append(comp, top)
			if top == b {
				break
			}
		}
		comps = append(comps, comp)
	}
	visit(entry)
	return comps, of
}

// cyclic holds for a component of more than one block, or of one that jumps to itself.
func cyclic(comp []*cfg.Block, of map[*cfg.Block]int, i int) bool {
	if len(comp) > 1 {
		return true
	}
	return slices.ContainsFunc(comp[0].Succs, func(s *cfg.Block) bool { return of[s] == i })
}

// name is the operation as the report shows it: package.Func or package.Type.Method.
func name(f *types.Func) string {
	if recv := f.Signature().Recv(); recv != nil {
		t := recv.Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if named, ok := types.Unalias(t).(*types.Named); ok {
			return f.Pkg().Name() + "." + named.Obj().Name() + "." + f.Name()
		}
	}
	return f.Pkg().Name() + "." + f.Name()
}
