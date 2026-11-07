package jobkind_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// declaredKeys names the package-level variables of jobkind.go initialised with a Key
// literal, read from the source so a new kind cannot be forgotten by this test.
func declaredKeys(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "jobkind.go", nil, 0)
	require.NoError(t, err)
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, value := range vs.Values {
				lit, ok := value.(*ast.CompositeLit)
				if !ok {
					continue
				}
				if ident, ok := lit.Type.(*ast.Ident); ok && ident.Name == "Key" {
					names = append(names, vs.Names[i].Name)
				}
			}
		}
	}
	return names
}

// Every kind declared with a purge key is listed in Purged, and each declares its
// argument: a kind added without being listed fails here.
func TestEveryDeclaredKeyIsPurgedAndNamesItsArgument(t *testing.T) {
	declared := declaredKeys(t)
	require.NotEmpty(t, declared)
	assert.Len(t, jobkind.Purged, len(declared), "Purged must list every declared Key: %v", declared)

	kinds := lo.Map(jobkind.Purged, func(k jobkind.Key, _ int) string { return k.Kind })
	assert.Len(t, lo.Uniq(kinds), len(kinds), "a kind is listed twice")
	for _, k := range jobkind.Purged {
		assert.NotEmpty(t, k.Arg, "kind %q has no declared purge key", k.Kind)
	}
}

// moduleRoot is the repository root, read from this package's directory.
const moduleRoot = "../../.."

// jobSources are the directories of the module that hold job args: the generated code and the
// end-to-end suite declare none.
var jobSources = []string{"cmd", "config", "ee", "internal"}

// instanceJobsWithArgs are the job args of no Workspace that still carry arguments, each with why
// it holds no Workspace's data. An args type with no field is a job of the Instance (a periodic
// sweep) and needs no entry.
var instanceJobsWithArgs = map[string]string{
	"internal/jobs.SendNoticeArgs": "a Notice to a User's own address (ADR 0105): account mail, gone with the User, not with a Workspace",
}

// workspaceEmbeds are the embedded types that name a job's Workspace at jobkind.WorkspaceArg, by
// the directory they are written in and how they are spelled there.
var workspaceEmbeds = map[string]bool{
	"jobkind.Workspace": true,
	"events.EventJob":   true,
	// events.EventJob, inside package events
	"internal/infra/events:EventJob": true,
}

// jobArgs is a type with a river Kind method, found in the source: its package directory, its name
// and its struct (nil when it is not a struct).
type jobArgs struct {
	dir, name string
	fields    *ast.FieldList
}

// declaredJobArgs reads every non-test Go file of jobSources and returns the types with a
// `Kind() string` method: river's JobArgs, the only types of the module that declare one.
func declaredJobArgs(t *testing.T) []jobArgs {
	t.Helper()
	kinds := map[string]bool{}
	structs := map[string]*ast.FieldList{}
	for _, src := range jobSources {
		err := filepath.WalkDir(filepath.Join(moduleRoot, src), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(moduleRoot, filepath.Dir(path))
			if err != nil {
				return err
			}
			dir := filepath.ToSlash(rel)
			collectJobArgs(file, dir, kinds, structs)
			return nil
		})
		require.NoError(t, err)
	}
	out := make([]jobArgs, 0, len(kinds))
	for key := range kinds {
		dir, name, _ := strings.Cut(key, ".")
		out = append(out, jobArgs{dir: dir, name: name, fields: structs[key]})
	}
	return out
}

// collectJobArgs adds the receivers of file's `Kind() string` methods to kinds and its struct
// types to structs, keyed by "<dir>.<name>".
func collectJobArgs(file *ast.File, dir string, kinds map[string]bool, structs map[string]*ast.FieldList) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if receiver, ok := kindReceiver(d); ok {
				kinds[dir+"."+receiver] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if st, ok := ts.Type.(*ast.StructType); ok {
					structs[dir+"."+ts.Name.Name] = st.Fields
				}
			}
		}
	}
}

// kindReceiver is the receiver type of a `func (T) Kind() string` method.
func kindReceiver(d *ast.FuncDecl) (string, bool) {
	if d.Recv == nil || len(d.Recv.List) != 1 || d.Name.Name != "Kind" || len(d.Type.Params.List) != 0 ||
		d.Type.Results == nil || len(d.Type.Results.List) != 1 {
		return "", false
	}
	if result, ok := d.Type.Results.List[0].Type.(*ast.Ident); !ok || result.Name != "string" {
		return "", false
	}
	recv := d.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	ident, ok := recv.(*ast.Ident)
	if !ok {
		return "", false
	}
	return ident.Name, true
}

// namesItsWorkspace reports whether a job's args carry its Workspace (an embedded
// jobkind.Workspace or events.EventJob) or carry nothing at all.
func namesItsWorkspace(args jobArgs) bool {
	if args.fields == nil {
		return false
	}
	if len(args.fields.List) == 0 {
		return true
	}
	return lo.SomeBy(args.fields.List, func(f *ast.Field) bool {
		if len(f.Names) != 0 {
			return false
		}
		switch typ := f.Type.(type) {
		case *ast.SelectorExpr:
			pkg, ok := typ.X.(*ast.Ident)
			return ok && workspaceEmbeds[pkg.Name+"."+typ.Sel.Name]
		case *ast.Ident:
			return workspaceEmbeds[args.dir+":"+typ.Name]
		}
		return false
	})
}

// Every river job of a Workspace names it at jobkind.WorkspaceArg, so the erase of the Workspace
// finds the job whatever its kind: a new kind whose args neither embed jobkind.Workspace (or an
// event job's envelope) nor carry nothing fails here, unless it is listed as the Instance's with
// its reason.
func TestEveryJobOfAWorkspaceNamesIt(t *testing.T) {
	declared := declaredJobArgs(t)
	require.NotEmpty(t, declared)

	unnamed := lo.FilterMap(declared, func(args jobArgs, _ int) (string, bool) {
		key := args.dir + "." + args.name
		_, instance := instanceJobsWithArgs[key]
		return key, !instance && !namesItsWorkspace(args)
	})

	assert.Empty(t, unnamed, "embed jobkind.Workspace in these job args, or list them in instanceJobsWithArgs with why no Workspace owns what they carry")
	listed := lo.Keys(instanceJobsWithArgs)
	declaredKeys := lo.Map(declared, func(args jobArgs, _ int) string { return args.dir + "." + args.name })
	assert.Subset(t, declaredKeys, listed, "instanceJobsWithArgs names job args that no longer exist")
}
