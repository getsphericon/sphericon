// Package closedenum enforces rule 19 of docs/standards/review-rules.md: a decision over a closed
// enum names every member, so a new member fails the lint wherever it is not handled.
//
// A closed enum is a named type of the list below; its members are the constants of that type its
// package declares. A switch whose tag is of the enum's type has no default clause and names every
// member in its cases, and a map literal keyed by it lists every member. A type declared from the
// enum (`type Medium channels.Channel`, or from such a type) is the enum too: the type, not a
// variable's name, decides. Go keeps no link from such a type to the enum it was declared from, so
// the package that declares it exports a fact on it naming the enum and its members, which the
// packages that switch on it read. A member is matched by its constant value, so a conversion of a
// member (`Medium(channels.Email)`) names it.
//
// Generated files are not read: a generator that maps an enum (goverter's enum mapping) checks its
// members itself when it runs, and its output is not edited by hand. Test files are not read either:
// a test decides nothing for the product, and rule 14 keeps switches out of test bodies.
package closedenum

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// enums are the closed enums the rule holds, by package path and type name. A new one is added
// here, with every switch and map literal on it brought to the rule in the same change.
var enums = []struct{ path, name string }{
	{path: tier.Module + "internal/kernel/channels", name: "Channel"},
}

// Analyzer reports a switch over a closed enum with a default clause or a member left out, and a
// map literal keyed by one that leaves a member out.
var Analyzer = &analysis.Analyzer{
	Name:      "closedenum",
	Doc:       "refuses a switch over a closed enum (channels.Channel) with a default or a member left out, and a map literal keyed by one that misses a member",
	Requires:  []*analysis.Analyzer{inspect.Analyzer},
	Run:       run,
	FactTypes: []analysis.Fact{new(Derived)},
}

// Enum is a closed enum as a decision over it needs it: its name as code writes it and its members.
type Enum struct {
	Name    string   // channels.Channel
	Members []Member // by name
}

// Member is a constant of a closed enum.
type Member struct {
	Name  string // channels.SMS
	Value string // its constant value, exactly: "sms"
}

// Derived is the fact on a type declared from a closed enum, directly or through another such type:
// it is that enum.
type Derived struct{ Enum Enum }

// AFact marks Derived as a fact.
func (*Derived) AFact() {}

func (d *Derived) String() string { return "derived from " + d.Enum.Name }

func run(pass *analysis.Pass) (any, error) {
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
	}
	exportDerived(pass, insp)
	for n := range insp.PreorderSeq((*ast.SwitchStmt)(nil), (*ast.CompositeLit)(nil)) {
		if skipped(pass, n) {
			continue
		}
		switch n := n.(type) {
		case *ast.SwitchStmt:
			checkSwitch(pass, n)
		case *ast.CompositeLit:
			checkMap(pass, n)
		}
	}
	return nil, nil
}

// exportDerived exports the fact on every type of the package declared from a closed enum. A type
// may be declared before the one it derives from, so the declarations are read until none is left
// to mark.
func exportDerived(pass *analysis.Pass, insp *inspector.Inspector) {
	var pending []*ast.TypeSpec
	for spec := range inspector.All[*ast.TypeSpec](insp) {
		if !spec.Assign.IsValid() {
			pending = append(pending, spec)
		}
	}
	for marked := true; marked; {
		marked = false
		rest := pending[:0]
		for _, spec := range pending {
			obj, ok := pass.TypesInfo.Defs[spec.Name].(*types.TypeName)
			if !ok {
				continue
			}
			e, isEnum := enumOf(pass, pass.TypesInfo.TypeOf(spec.Type))
			if !isEnum {
				rest = append(rest, spec)
				continue
			}
			pass.ExportObjectFact(obj, &Derived{Enum: e})
			marked = true
		}
		pending = rest
	}
}

// skipped holds for a node in a generated file or a test file.
func skipped(pass *analysis.Pass, n ast.Node) bool {
	if analysisx.InTestFile(pass, n.Pos()) {
		return true
	}
	for _, f := range pass.Files {
		if f.FileStart <= n.Pos() && n.Pos() < f.FileEnd {
			return ast.IsGenerated(f)
		}
	}
	return false
}

// checkSwitch reports a switch over a closed enum that has a default clause or leaves a member out.
func checkSwitch(pass *analysis.Pass, sw *ast.SwitchStmt) {
	if sw.Tag == nil {
		return
	}
	e, ok := enumOf(pass, pass.TypesInfo.TypeOf(sw.Tag))
	if !ok {
		return
	}
	var cases []ast.Expr
	for _, stmt := range sw.Body.List {
		clause, isClause := stmt.(*ast.CaseClause)
		if !isClause {
			continue
		}
		if clause.List == nil {
			rule.Report(pass, sw.Pos(), 19,
				fmt.Sprintf("the switch on %s has a default clause, which a new member would take unnoticed", e.Name),
				"drop the default and name every member in a case, a member the switch does not apply to in a case that says what happens to it (an error, a refusal, nothing); a value outside the set is handled after the switch")
			return
		}
		cases = append(cases, clause.List...)
	}
	if missing := e.missing(pass, cases); len(missing) > 0 {
		rule.Report(pass, sw.Pos(), 19,
			fmt.Sprintf("the switch on %s does not name %s", e.Name, strings.Join(missing, ", ")),
			"add a case for each, one the switch does not apply to in a case that says what happens to it (an error, a refusal, nothing)")
	}
}

// checkMap reports a map literal keyed by a closed enum that leaves a member out. An empty literal
// is no table, only a map to fill.
func checkMap(pass *analysis.Pass, lit *ast.CompositeLit) {
	if len(lit.Elts) == 0 {
		return
	}
	m, ok := pass.TypesInfo.TypeOf(lit).Underlying().(*types.Map)
	if !ok {
		return
	}
	e, ok := enumOf(pass, m.Key())
	if !ok {
		return
	}
	var keys []ast.Expr
	for _, elt := range lit.Elts {
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			keys = append(keys, kv.Key)
		}
	}
	if missing := e.missing(pass, keys); len(missing) > 0 {
		rule.Report(pass, lit.Pos(), 19,
			fmt.Sprintf("the map keyed by %s does not list %s", e.Name, strings.Join(missing, ", ")),
			"list every member; a table that applies to some members only is a function with a switch that names every member")
	}
}

// missing is the name of every member of e that none of exprs names by its constant value.
func (e Enum) missing(pass *analysis.Pass, exprs []ast.Expr) []string {
	named := map[string]bool{}
	for _, x := range exprs {
		if tv, ok := pass.TypesInfo.Types[x]; ok && tv.Value != nil {
			named[tv.Value.ExactString()] = true
		}
	}
	var missing []string
	for _, m := range e.Members {
		if !named[m.Value] {
			missing = append(missing, m.Name)
		}
	}
	return missing
}

// enumOf is the closed enum t is, under any alias: a type of the list, or a type the Derived fact
// marks.
func enumOf(pass *analysis.Pass, t types.Type) (Enum, bool) {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return Enum{}, false
	}
	obj := named.Obj()
	for _, e := range enums {
		if obj.Pkg() != nil && obj.Pkg().Path() == e.path && obj.Name() == e.name {
			return listed(obj), true
		}
	}
	var d Derived
	if pass.ImportObjectFact(obj, &d) {
		return d.Enum, true
	}
	return Enum{}, false
}

// listed is the enum of the list whose type is obj, with the constants of its type that its package
// declares.
func listed(obj *types.TypeName) Enum {
	pkg := obj.Pkg()
	e := Enum{Name: pkg.Name() + "." + obj.Name()}
	for _, name := range pkg.Scope().Names() {
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if ok && types.Identical(c.Type(), obj.Type()) {
			e.Members = append(e.Members, Member{Name: pkg.Name() + "." + name, Value: c.Val().ExactString()})
		}
	}
	return e
}
