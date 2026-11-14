package entgen_test

import (
	"path/filepath"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/schema"
	"github.com/getsphericon/sphericon/internal/entgen"
)

// load classifies the schema of the testdata package dir as the generation does.
func load(t *testing.T, dir string) (*entgen.Schema, []entgen.Problem) {
	t.Helper()
	graph, err := entc.LoadGraph(dir, entgen.Config())
	require.NoError(t, err)
	return entgen.Classify(graph)
}

// where names a problem by what it is about and the rule it breaks.
func where(p entgen.Problem) string {
	return p.Where() + " " + string(p.Rule)
}

// Each rule of the generation has a schema that breaks it, and the check names every problem in it,
// not only the first.
func TestCheckFindsEveryProblem(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want []string
	}{
		{"archivedanddiscarded", []string{"Thing archived-and-discarded"}},
		{"markerwithoutmixin", []string{
			"Owned marker-without-mixin", "Owned marker-without-mixin", "Aside marker-without-mixin", "Gone marker-without-mixin",
		}},
		{"versionedkind", []string{"Definition versioned-kind"}},
		{"versionededges", []string{"Thing versioned-live-edge", "Thing versioned-owner"}},
		{"versionedowner", []string{"Definition versioned-owner"}},
		{"versionedfields", []string{"Definition versioned-fields"}},
		{"enumwithoutcheck", []string{"Unchecked.color enum-without-check", "Misnamed.shade enum-without-check"}},
		{"givenupnotbool", []string{"Thing.label given-up-not-bool"}},
		{"givenupwithoutdiscard", []string{"Thing.is_default given-up-without-discard"}},
		{"bookkeepingwithoutrevision", []string{"Thing.polls bookkeeping-without-revision"}},
		{"uncheckedrefreason", []string{"Thing.gone_id unchecked-ref-reason"}},
		{"refwithoutkey", []string{"Thing.other_id ref-without-key"}},
		{"refunknownentity", []string{"Thing.ghost_id ref-unknown-entity"}},
		{"auditaction", []string{"Thing audit-action"}},
		{"auditnamefield", []string{"Thing audit-name-field"}},
		{"auditalways", []string{"Thing audit-always"}},
		{"statewithoutform", []string{
			"Thing.enabled state-without-form", "Thing.seen_at state-without-form", "Thing.status state-without-form",
		}},
		{"stateunknownform", []string{"Thing.enabled state-unknown-form"}},
		{"attributereason", []string{"Thing.seen_at attribute-reason"}},
		{"uniquenotpartial", []string{
			"Thing.code unique-not-partial", "Thing.token unique-not-partial", "Thing index things_name unique-not-partial",
		}},
		{"workspaceoutsidemixin", []string{
			"Journal.workspace_id workspace-outside-mixin", "Trace.workspace_id workspace-outside-mixin",
			"Claim.workspace_id workspace-outside-mixin", "Claim marker-without-mixin",
		}},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			_, problems := load(t, "./"+filepath.Join("testdata", tc.dir))

			found := lo.Map(problems, func(p entgen.Problem, _ int) string { return where(p) })
			assert.ElementsMatch(t, tc.want, found)
		})
	}
}

// Every entity is classified by the markers of its mixins, and a singleton by a whole unique index
// on its Workspace column alone.
func TestClassifyGivesEachEntityItsKind(t *testing.T) {
	s, problems := load(t, "./"+filepath.Join("testdata", "kinds"))
	require.Empty(t, problems)

	kinds := lo.SliceToMap(s.Entities(), func(e entgen.Entity) (string, schema.Kind) { return e.Name, e.Kind })
	assert.Equal(t, map[string]schema.Kind{
		"Workspace":         {},
		"Plain":             {},
		"Owned":             {WorkspaceOwned: true},
		"Archived":          {WorkspaceOwned: true, Archivable: true},
		"Discarded":         {WorkspaceOwned: true, Discardable: true, Revised: true},
		"Single":            {WorkspaceOwned: true, Singleton: true},
		"PartialSingle":     {WorkspaceOwned: true},
		"Definition":        {WorkspaceOwned: true, Archivable: true, Versioned: true, Revised: true},
		"DefinitionVersion": {WorkspaceOwned: true},
	}, kinds)
}

// The real schema breaks no rule of the generation, and its entities have the kinds the product
// gives them.
func TestTheSchemaBreaksNoRule(t *testing.T) {
	s, problems := load(t, filepath.Join("..", "..", "ent", "schema"))

	require.Empty(t, lo.Map(problems, func(p entgen.Problem, _ int) string { return p.String() }))
	entities := lo.SliceToMap(s.Entities(), func(e entgen.Entity) (string, entgen.Entity) { return e.Name, e })
	assert.Equal(t, "automations", entities["Automation"].Table)
	assert.Equal(t, schema.Kind{WorkspaceOwned: true, Archivable: true, Versioned: true, Revised: true}, entities["Automation"].Kind)
	assert.Equal(t, schema.Kind{WorkspaceOwned: true, Discardable: true, Versioned: true, Revised: true}, entities["Rule"].Kind)
	assert.Equal(t, schema.Kind{WorkspaceOwned: true, Revised: true, Singleton: true}, entities["BrandKit"].Kind)
	assert.Equal(t, schema.Kind{WorkspaceOwned: true, Singleton: true}, entities["SendCapLimiter"].Kind)
	assert.Equal(t, schema.Kind{WorkspaceOwned: true}, entities["OAuthCode"].Kind)
	assert.Equal(t, schema.Kind{Discardable: true, Revised: true}, entities["Workspace"].Kind)
	assert.Equal(t, schema.Kind{}, entities["EventJournalEntry"].Kind)
}

// An entity's writes are named by the verbs of its kind, declared once: every Workspace-owned entity
// creates, updates and deletes, a discardable one also discards and an archivable one archives and
// unarchives (ADR 0100). A template that names a verb the entity does not have fails the generation.
func TestEachKindHasTheVerbsOfItsWrites(t *testing.T) {
	graph, err := entc.LoadGraph("./"+filepath.Join("testdata", "kinds"), entgen.Config())
	require.NoError(t, err)
	s, problems := entgen.Classify(graph)
	require.Empty(t, problems)

	verbs := lo.SliceToMap(s.Entities(), func(e entgen.Entity) (string, []string) { return e.Name, entgen.Verbs(e.Kind) })
	assert.Equal(t, []string{"create", "update", "delete"}, verbs["Owned"])
	assert.Equal(t, []string{"create", "update", "delete", "archive", "unarchive"}, verbs["Archived"])
	assert.Equal(t, []string{"create", "update", "delete", "discard"}, verbs["Discarded"])

	verb, ok := s.Funcs()["verb"].(func(*gen.Type, string) (string, error))
	require.True(t, ok, "the templates name a verb through verb")
	nodes := lo.SliceToMap(graph.Nodes, func(n *gen.Type) (string, *gen.Type) { return n.Name, n })
	got, err := verb(nodes["Archived"], "archive")
	require.NoError(t, err)
	assert.Equal(t, "archive", got)
	_, err = verb(nodes["Owned"], "archive")
	assert.ErrorContains(t, err, `Owned has no verb "archive"`)
}
