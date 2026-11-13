package schema_test

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

// The manifest of roles (ent/template/roles.tmpl) is what the analyzers of tools/analyzers know of
// the generated ent package: what each type is, the entity it belongs to, the role of each method
// they ask about and the state field an update writes. These tests hold it complete against the
// package itself, and hold the analyzers' stand-ins of the package to the same rules.

// The files ent/template writes: one per template it defines, its helpers aside.
func emittedFiles(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("../template/*.tmpl")
	require.NoError(t, err)
	define := regexp.MustCompile(`\{\{-? define "([^"]+)" -?\}\}`)
	files := map[string]bool{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, m := range define.FindAllStringSubmatch(string(raw), -1) {
			if !strings.HasPrefix(m[1], "helper/") {
				files[m[1]+".go"] = true
			}
		}
	}
	return files
}

// generated loads the generated ent package from its source, so every declaration keeps its file.
func generated(t *testing.T) (*token.FileSet, *types.Package) {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo, Dir: ".."}
	pkgs, err := packages.Load(cfg, ".")
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	require.Empty(t, pkgs[0].Errors)
	return pkgs[0].Fset, pkgs[0].Types
}

// standIn type-checks a stand-in of the ent package that an analyzer's test data carries.
func standIn(t *testing.T, dir string) (*token.FileSet, *types.Package) {
	t.Helper()
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("github.com/getsphericon/sphericon/ent", fset, files, nil)
	require.NoError(t, err)
	return fset, pkg
}

// manifest is what a package declares about itself, read from its constants.
type manifest struct {
	pkg    *types.Package
	values map[string]string // every string constant of the package by name
}

func read(pkg *types.Package) manifest {
	m := manifest{pkg: pkg, values: map[string]string{}}
	for _, name := range pkg.Scope().Names() {
		if c, ok := pkg.Scope().Lookup(name).(*types.Const); ok && c.Val().Kind() == constant.String {
			m.values[name] = constant.StringVal(c.Val())
		}
	}
	return m
}

// vocabulary lists the values of the constants named prefix<Word>, the words a family may take.
func (m manifest) vocabulary(prefix string) []string {
	var out []string
	for name, v := range m.values {
		if rest, ok := strings.CutPrefix(name, prefix); ok && rest != "" && !strings.Contains(rest, "_") {
			out = append(out, v)
		}
	}
	return out
}

// entries lists the constants of a family (TypeOf_, RoleOf_, ...) by the rest of their name.
func (m manifest) entries(family string) map[string]string {
	out := map[string]string{}
	for name, v := range m.values {
		if rest, ok := strings.CutPrefix(name, family); ok {
			out[rest] = v
		}
	}
	return out
}

// named is the type of the package with the name, or nil: an alias names another package's type.
func (m manifest) named(name string) *types.Named {
	tn, ok := m.pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok || tn.IsAlias() {
		return nil
	}
	n, _ := tn.Type().(*types.Named)
	return n
}

// methods lists the methods declared on a type, or on the interface it names.
func methods(n *types.Named) []*types.Func {
	if iface, ok := n.Underlying().(*types.Interface); ok {
		return slices.Collect(iface.ExplicitMethods())
	}
	return slices.Collect(n.Methods())
}

func (m manifest) method(typeName, name string) *types.Func {
	n := m.named(typeName)
	if n == nil {
		return nil
	}
	for _, fn := range methods(n) {
		if fn.Name() == name {
			return fn
		}
	}
	return nil
}

// split cuts "<Type>_<Method>".
func split(key string) (typ, method string) {
	typ, method, _ = strings.Cut(key, "_")
	return typ, method
}

// problems lists every way the package and its manifest disagree: an exported type with no TypeOf_
// or EntityOf_, an exported method declared in a file a template emits with no RoleOf_, an entry
// whose type or method does not exist or whose value is not a word of its vocabulary, and a state
// write that names no state field of its entity or sits on a builder that changes no existing row.
func problems(fset *token.FileSet, pkg *types.Package, emitted map[string]bool) []string {
	m := read(pkg)
	out := slices.Concat(m.undescribed(fset, emitted), m.typeEntries(), m.roleEntries(), m.stateEntries())
	slices.Sort(out)
	return out
}

// undescribed lists the exported types without their entries and the exported methods a template
// emits without their role.
func (m manifest) undescribed(fset *token.FileSet, emitted map[string]bool) []string {
	var out []string
	for _, name := range m.pkg.Scope().Names() {
		n := m.named(name)
		if n == nil || !n.Obj().Exported() {
			continue
		}
		for _, family := range []string{"TypeOf_", "EntityOf_"} {
			if _, ok := m.values[family+name]; !ok {
				out = append(out, "type "+name+" has no "+family+name)
			}
		}
		for _, fn := range methods(n) {
			_, ok := m.values["RoleOf_"+name+"_"+fn.Name()]
			if fn.Exported() && emitted[filepath.Base(fset.Position(fn.Pos()).Filename)] && !ok {
				out = append(out, "method "+name+"."+fn.Name()+" has no RoleOf_"+name+"_"+fn.Name())
			}
		}
	}
	return out
}

// typeEntries lists the TypeOf_ and EntityOf_ entries that name no type or hold no word of their
// vocabulary.
func (m manifest) typeEntries() []string {
	var out []string
	typeOf, words := m.entries("TypeOf_"), m.vocabulary("Type")
	for name, v := range typeOf {
		if m.named(name) == nil {
			out = append(out, "TypeOf_"+name+" names no type")
		}
		if !slices.Contains(words, v) {
			out = append(out, "TypeOf_"+name+" is "+v+", no Type word")
		}
	}
	for name, v := range m.entries("EntityOf_") {
		if m.named(name) == nil {
			out = append(out, "EntityOf_"+name+" names no type")
		}
		if v != "" && typeOf[v] != "entity" {
			out = append(out, "EntityOf_"+name+" is "+v+", no entity")
		}
	}
	return out
}

// roleEntries lists the RoleOf_ entries that name no method or hold no Role word.
func (m manifest) roleEntries() []string {
	var out []string
	roles := m.vocabulary("Role")
	for key, v := range m.entries("RoleOf_") {
		if m.method(split(key)) == nil {
			out = append(out, "RoleOf_"+key+" names no method")
		}
		if !slices.Contains(roles, v) {
			out = append(out, "RoleOf_"+key+" is "+v+", no Role word")
		}
	}
	return out
}

// changesRows are the kinds of builder that change an existing row, the ones a state write sits on.
var changesRows = []string{"update", "upsert", "scoped update", "scoped upsert"}

// stateEntries lists the state writes that name no method, no state field of their entity, or sit
// on a builder that changes no existing row.
func (m manifest) stateEntries() []string {
	var out []string
	typeOf, entityOf := m.entries("TypeOf_"), m.entries("EntityOf_")
	for _, family := range []string{"SetsState_", "ClearsState_"} {
		for key, field := range m.entries(family) {
			typ, _ := split(key)
			if m.method(split(key)) == nil {
				out = append(out, family+key+" names no method")
			}
			if _, ok := m.values["StateForm"+entityOf[typ]+field]; !ok {
				out = append(out, family+key+" names "+field+", no state field of "+entityOf[typ])
			}
			if !slices.Contains(changesRows, typeOf[typ]) {
				out = append(out, family+key+" is on "+typ+", which changes no existing row")
			}
		}
	}
	return out
}

// unlistedStateWrites lists the methods of an update or upsert builder that set or clear a state
// field and that neither SetsState_ nor ClearsState_ names: the generator's list held to the methods.
func unlistedStateWrites(pkg *types.Package) []string {
	m := read(pkg)
	typeOf, entityOf := m.entries("TypeOf_"), m.entries("EntityOf_")
	listed := map[string]bool{}
	for _, family := range []string{"SetsState_", "ClearsState_"} {
		for key := range m.entries(family) {
			listed[key] = true
		}
	}
	var out []string
	for typ, what := range typeOf {
		if !slices.Contains(changesRows, what) {
			continue
		}
		for name := range m.values {
			field, ok := strings.CutPrefix(name, "StateForm"+entityOf[typ])
			if !ok || field == "" {
				continue
			}
			for _, verb := range []string{"Set", "SetNillable", "Clear", "Update"} {
				if m.method(typ, verb+field) != nil && !listed[typ+"_"+verb+field] {
					out = append(out, typ+"."+verb+field)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// entityUpdates lists the types whose Update returns their own <T>UpdateOne: a loaded entity's update,
// which bypasses the Workspace scope (ADR 0017), and the manifest's entities.
func entityUpdates(pkg *types.Package) (byShape, byManifest []string) {
	m := read(pkg)
	for _, name := range pkg.Scope().Names() {
		fn := m.method(name, "Update")
		if fn == nil || fn.Signature().Results().Len() != 1 {
			continue
		}
		if p, ok := fn.Signature().Results().At(0).Type().(*types.Pointer); ok && types.TypeString(p.Elem(), types.RelativeTo(pkg)) == name+"UpdateOne" {
			byShape = append(byShape, name)
		}
	}
	for name, what := range m.entries("TypeOf_") {
		if what == "entity" && m.values["RoleOf_"+name+"_Update"] == "start" {
			byManifest = append(byManifest, name)
		}
	}
	slices.Sort(byManifest)
	return byShape, byManifest
}

func TestTheManifestOfRolesCoversTheGeneratedPackage(t *testing.T) {
	fset, pkg := generated(t)

	assert.Empty(t, problems(fset, pkg, emittedFiles(t)))
	assert.Empty(t, unlistedStateWrites(pkg))
}

// The entity Update rule (tools/analyzers/entityupdate) reads the entities from the manifest; they
// are exactly the types whose Update returns their own update builder, OAuthClient among them,
// whose name ends in Client.
func TestTheManifestNamesEveryEntityWhoseUpdateBypassesTheScope(t *testing.T) {
	_, pkg := generated(t)

	byShape, byManifest := entityUpdates(pkg)

	require.GreaterOrEqual(t, len(byShape), 25)
	assert.Contains(t, byShape, "OAuthClient")
	assert.Equal(t, byShape, byManifest)
}

// standInProblems lists the problems of each stand-in, one list per directory.
func standInProblems(t *testing.T, dirs []string) [][]string {
	t.Helper()
	emitted := emittedFiles(t)
	found := make([][]string, len(dirs))
	for i, dir := range dirs {
		fset, pkg := standIn(t, dir)
		found[i] = append(problems(fset, pkg, emitted), unlistedStateWrites(pkg)...)
	}
	return found
}

// The analyzers' tests run on stand-ins of the ent package; each carries its manifest, held to the
// same rules.
func TestTheAnalyzersStandInsCarryTheirManifest(t *testing.T) {
	dirs := []string{
		"../../tools/analyzers/entrywrite/testdata/src/github.com/getsphericon/sphericon/ent",
		"../../tools/analyzers/statemoment/testdata/src/github.com/getsphericon/sphericon/ent",
		"../../tools/analyzers/entityupdate/testdata/src/github.com/getsphericon/sphericon/ent",
	}

	assert.Equal(t, make([][]string, len(dirs)), standInProblems(t, dirs))
}
