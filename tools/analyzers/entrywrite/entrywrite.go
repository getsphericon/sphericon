// Package entrywrite enforces rule 5 of docs/standards/review-rules.md for writes, in two halves.
//
// Only the owner writes a table (ADR 0102): `owners` names, for each ent entity, the package that
// owns its table, and a write to the entity from any other package of the module is a finding. A
// write to an entity the map does not name is a finding too, so a new entity names its owner in the
// change that adds it (a test checks the map against ent/schema). A write is a call that starts an
// ent mutation (Create, CreateBulk, MapCreateBulk, Update, UpdateOne, UpdateOneID, Delete, DeleteOne,
// DeleteOneID), an upsert (OnConflict, OnConflictColumns, OnConflictColumnsWhere), a move of the
// soft-delete mixins (Archive, Unarchive, Discard, Purge, PurgeOneID) or of a Versioned definition's
// pointers (Pointers and the moves on them), on a type of the ent package: the raw client, a
// transaction and the generated scoped wrappers. A generic function instantiated with an ent write
// builder (`stepOf[*ent.TagScopedDelete]`) writes that builder's entity where it is instantiated. The
// moves of `ent.VersionedStore` are not read: the store is the owner's Pointers, bound by the owner
// (counted there) and handed to the lifecycle in internal/drafts. Generated code (ent/) and test code
// are not read: test files, the test harness (internal/testhelper, which only tests import) and the
// end-to-end suite (e2e/, behind its build tag).
//
// An entry point (a handler of any surface, an MCP tool, a job or an event subscriber) receives input
// and calls a domain module or a use case; it owns no table, so every write there is a finding, worded
// for the entry point. The handlers of the API surfaces (`surfaces`) do not read through ent either: a
// call that starts a read (Get, GetX, a Query method, or Workspace on the scoped client) on a type of
// the ent package is a finding there, and the read goes into the module that owns the entity. The other
// entry points may read. Nor does such a handler open a transaction (a Within… method of the events
// bus): the module does.
package entrywrite

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
)

const module = "github.com/getsphericon/sphericon/"

// eventsPath is the package of the events bus, whose Within… methods open a transaction.
const eventsPath = module + "internal/infra/events"

// entryPoints are the packages that receive input from outside (an API surface, MCP, a queue, the
// ingest hooks) and hand it to a domain module.
var entryPoints = []string{
	"internal/api/site",
	"internal/api/external",
	"internal/api/collect",
	"internal/api/staff",
	"ee/api/staff",
	"internal/mcpserver",
	"internal/jobs",
	"internal/server",
}

// surfaces are the entry points that read through a domain module too: the handlers of the API surfaces.
var surfaces = []string{
	"internal/api/site",
	"internal/api/external",
	"internal/api/collect",
	"internal/api/staff",
	"ee/api/staff",
}

// owners names, for each ent entity, the package that owns its table: the only one that writes it
// (ADR 0102). A domain module owns the tables of its domain; the infrastructure owns the few that
// belong to no domain (the Events of the bus, the Login throttle's attempts, the schema state the
// migrations keep); a hook package owns the log it keeps; the Enterprise Edition owns its own.
var owners = map[string]string{
	"ApiToken":             "internal/apitokens",
	"AuditEntry":           "ee/audit",
	"AuthAttempt":          "internal/infra/throttle",
	"Automation":           "internal/automations",
	"AutomationRun":        "internal/automations",
	"AutomationVersion":    "internal/automations",
	"Binding":              "internal/bindings",
	"BrandKit":             "internal/brandkit",
	"Broadcast":            "internal/broadcasts",
	"BroadcastRecipient":   "internal/broadcasts",
	"CapturedMessage":      "internal/capturedmessages",
	"Confirmation":         "internal/confirmations",
	"ConsentRecord":        "internal/consentrecords",
	"Contact":              "internal/contacts",
	"ContactTag":           "internal/contacts",
	"CustomField":          "internal/contacts",
	"EmailTemplate":        "internal/templates",
	"Erasure":              "ee/export",
	"Event":                "internal/infra/events",
	"HeldOrderEvent":       "internal/orders",
	"Hold":                 "internal/holds",
	"InstanceAuditEntry":   "internal/instanceaudit",
	"InstanceState":        "internal/instancestates",
	"Integration":          "internal/integrations",
	"Invitation":           "internal/invitations",
	"JoinDomain":           "internal/joindomains",
	"LegalDocumentDraft":   "internal/signupitems",
	"LegalDocumentVersion": "internal/signupitems",
	"Membership":           "internal/memberships",
	"OAuthClient":          "internal/oauthclients",
	"OAuthCode":            "internal/oauthclients",
	"OAuthRefreshToken":    "internal/oauthclients",
	"Order":                "internal/orders",
	"OrderMovement":        "internal/orders",
	"OutboundMessage":      "internal/outbound",
	"Passkey":              "internal/passkeys",
	"PasskeyChallenge":     "internal/passkeys",
	"PendingSignup":        "internal/users",
	"PrivateRelayRange":    "internal/machine",
	"ProviderLimiter":      "internal/providerlimit",
	"RecoveryCode":         "internal/recoverycodes",
	"Rule":                 "internal/rules",
	"RuleFiring":           "internal/rules",
	"RuleVersion":          "internal/rules",
	"RuleVersionTrigger":   "internal/rules",
	"SchemaState":          "internal/infra/db",
	"Segment":              "internal/segments",
	"SendCapLimiter":       "internal/providerlimit",
	"SendingDomain":        "internal/sendingdomains",
	"ServiceCredential":    "ee/staff",
	"SignupItem":           "internal/signupitems",
	"SignupSurveyResponse": "internal/signupsurvey",
	"StaffMember":          "internal/staffmembers",
	"StartToken":           "internal/bindings",
	"Suppression":          "internal/suppression",
	"Tag":                  "internal/tags",
	"TrackedLink":          "internal/outbound",
	"TrackingDomain":       "internal/trackingdomains",
	"TrackingHostChange":   "internal/trackinghostlog",
	"Unsubscribe":          "internal/unsubscribes",
	"Usage":                "ee/usage",
	"User":                 "internal/users",
	"UserIdentity":         "internal/users",
	"UserTombstone":        "internal/userfeed",
	"Visitor":              "internal/visitors",
	"WebhookEndpoint":      "internal/webhooks",
	"Workspace":            "internal/workspaces",
}

// Owners returns a copy of the ownership map: each ent entity and the package that owns its table.
func Owners() map[string]string { return maps.Clone(owners) }

// unread are the packages of the module the analyzer does not read: the generated ent package, and
// the test code outside `_test.go` files: the test harness, which only tests import, and the
// end-to-end suite behind its build tag.
var unread = []string{"ent", "internal/testhelper", "e2e"}

// Analyzer reports ent writes outside the owner of the table and in entry-point packages.
var Analyzer = &analysis.Analyzer{
	Name:     "entrywrite",
	Doc:      "refuses a write to an ent entity outside the package that owns its table, and any write through ent in handlers, MCP tools, jobs and event subscribers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var starters = map[string]bool{
	"Create": true, "CreateBulk": true, "MapCreateBulk": true,
	"Update": true, "UpdateOne": true, "UpdateOneID": true,
	"Delete": true, "DeleteOne": true, "DeleteOneID": true,
	// The soft-delete mixins (ADR 0100).
	"Archive": true, "Unarchive": true, "Discard": true, "Purge": true, "PurgeOneID": true,
	// A Versioned definition's pointers (ADR 0065): the handle and its moves.
	"Pointers": true, "SetDraft": true, "ClearDraft": true, "Promote": true,
	"Activate": true, "Deactivate": true, "DeleteVersion": true,
}

// builders are the suffixes of an ent write builder's type after its entity: a generic function
// instantiated with one writes that entity.
var builders = map[string]bool{
	"Create": true, "CreateBulk": true, "Update": true, "UpdateOne": true, "Delete": true, "DeleteOne": true,
	"Upsert": true, "UpsertOne": true, "UpsertBulk": true, "Pointers": true,
}

var upserts = map[string]bool{"OnConflict": true, "OnConflictColumns": true, "OnConflictColumnsWhere": true}

func isIn(path, pkg string) bool {
	return path == module+pkg || strings.HasPrefix(path, module+pkg+"/")
}

func isEntryPoint(path string) bool {
	return slices.ContainsFunc(entryPoints, func(p string) bool { return isIn(path, p) })
}

func isSurface(path string) bool {
	return slices.ContainsFunc(surfaces, func(p string) bool { return isIn(path, p) })
}

func isRead(path string) bool {
	return strings.HasPrefix(path, module) && !slices.ContainsFunc(unread, func(p string) bool { return isIn(path, p) })
}

// startsRead reports whether a method of the ent package named name starts a read: the generated
// Get and GetX, a query (Query, QueryWithDiscarded, QueryArchived, an edge's QueryX) and the scoped
// client's Workspace.
func startsRead(name, named string) bool {
	return name == "Get" || name == "GetX" || strings.HasPrefix(name, "Query") ||
		(name == "Workspace" && named == "Scoped")
}

// checker holds what a package's findings depend on: its path, whether it is an entry point and
// whether it is an API surface's handlers.
type checker struct {
	pass  *analysis.Pass
	path  string
	entry bool
	reads bool
}

func run(pass *analysis.Pass) (any, error) {
	path := pass.Pkg.Path()
	if !isRead(path) {
		return nil, nil
	}
	c := checker{pass: pass, path: path, entry: isEntryPoint(path), reads: isSurface(path)}
	insp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, fmt.Errorf("%s: the inspect analyzer gave no *inspector.Inspector", pass.Analyzer.Name)
	}
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil), (*ast.Ident)(nil)}, func(n ast.Node) {
		if strings.HasSuffix(pass.Fset.Position(n.Pos()).Filename, "_test.go") {
			return
		}
		switch n := n.(type) {
		case *ast.CallExpr:
			c.call(n)
		case *ast.Ident:
			c.instance(n)
		}
	})
	return nil, nil
}

func (c checker) call(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if c.reads && opensTx(c.pass.TypesInfo, sel) {
		c.pass.Reportf(call.Pos(), "%s", rule.Message(5,
			"a handler opens a transaction ("+sel.Sel.Name+" on the events bus) and holds logic a domain module owns",
			"move the transaction and what runs in it into the domain module and call that module from here"))
		return
	}
	_, named, ok := entx.Method(c.pass.TypesInfo, sel)
	if !ok {
		return
	}
	if c.reads && startsRead(sel.Sel.Name, named.Obj().Name()) {
		entity := entx.Entity(named)
		if entity == "Scoped" {
			entity = "Workspace"
		}
		c.pass.Reportf(call.Pos(), "%s", rule.Message(5,
			"a handler reads through ent ("+sel.Sel.Name+" on "+entity+") instead of asking a domain module",
			"give the domain module that owns the read a function for it (a Get or the operation's own read) and call that from here"))
		return
	}
	if (!starters[sel.Sel.Name] && !upserts[sel.Sel.Name]) || chainStartsWrite(sel.X) {
		return
	}
	if named.Obj().Name() == "VersionedStore" {
		return
	}
	entity := entx.Entity(named)
	c.write(call.Pos(), sel.Sel.Name+" on "+entity, entity)
}

// instance reads an instantiation of a generic function or type: a type argument that is an ent
// write builder makes it a write of that builder's entity.
func (c checker) instance(id *ast.Ident) {
	inst, ok := c.pass.TypesInfo.Instances[id]
	if !ok {
		return
	}
	for t := range inst.TypeArgs.Types() {
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		named, ok := t.(*types.Named)
		if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != entx.Path {
			continue
		}
		entity := entx.Entity(named)
		suffix := strings.TrimPrefix(strings.TrimPrefix(named.Obj().Name(), entity), "Scoped")
		if entity == named.Obj().Name() || !builders[suffix] {
			continue
		}
		c.write(id.Pos(), id.Name+" with "+named.Obj().Name(), entity)
		return
	}
}

// write reports a write to entity, worded by what is: how it writes, made by the package checked.
func (c checker) write(pos token.Pos, what, entity string) {
	if c.entry {
		c.pass.Reportf(pos, "%s", rule.Message(5,
			"an entry point writes through ent ("+what+") and holds logic a domain module owns",
			"move the write into the domain module that owns "+entity+" and call that module from here"))
		return
	}
	owner, ok := owners[entity]
	if !ok {
		c.pass.Reportf(pos, "%s", rule.Message(5,
			"a write to "+entity+" ("+what+"), an entity the ownership map names no owner of",
			"add "+entity+" to owners in tools/analyzers/entrywrite with the domain module that owns its table"))
		return
	}
	if isIn(c.path, owner) {
		return
	}
	pkg := strings.TrimPrefix(c.path, module)
	c.pass.Reportf(pos, "%s", rule.Message(5,
		pkg+" writes "+entity+" ("+what+"), a table "+owner+" owns",
		"give "+owner+" a function that makes the write and call it from here (ADR 0102: only the owner writes a table)"))
}

// opensTx reports whether the selector names a method of the events bus that opens a transaction.
func opensTx(info *types.Info, sel *ast.SelectorExpr) bool {
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Signature().Recv() == nil || fn.Pkg() == nil || fn.Pkg().Path() != eventsPath {
		return false
	}
	return strings.HasPrefix(fn.Name(), "Within")
}

// chainStartsWrite reports whether the receiver chain already holds a call that starts a write, so
// that `Create().OnConflict...` and `Pointers().SetDraft(...)` are one finding each, and an upsert on
// a builder variable is another.
func chainStartsWrite(e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if starters[sel.Sel.Name] {
			return true
		}
		e = sel.X
	}
}
