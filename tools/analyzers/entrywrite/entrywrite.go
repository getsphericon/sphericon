// Package entrywrite enforces rule 5 of docs/standards/review-rules.md for writes, in two halves.
//
// Only the owner writes a table (ADR 0102): `owners` names, for each ent entity, the package that
// owns its table, and a write to the entity from any other package of the module is a finding. A
// write to an entity the map does not name is a finding too, so a new entity names its owner in the
// change that adds it (a test checks the map against ent/schema). What a call does is the generator's
// to say, not a name's: the manifest of roles of the ent package (ent/template/roles.tmpl, read
// through entx) gives every method the analyzer asks about its role and every type its kind and
// entity. A write is a call of a method whose role starts a write (a create, an update, a delete, an
// upsert, a move of the soft-delete mixins or of a Versioned definition's pointers), counted once
// per receiver chain: `Create().OnConflictColumns(...)` is one write. A generic function instantiated
// with a type whose kind builds a write (`stepOf[*ent.TagScopedDelete]`) writes that type's entity
// where it is instantiated. The moves of `ent.VersionedStore` carry on a write already started: the
// store is the owner's Pointers, bound by the owner (counted there) and handed to the lifecycle in
// internal/domain/content/versioneddefinition.
// Generated code (ent/) and test code are not read: test files, the test harness
// (internal/testhelper, which only tests import) and the end-to-end suite (e2e/, behind its build
// tag).
//
// The Workspace erase (ADR 0110) is the one writer of a Workspace-owned table besides its owner: the
// rows of a Workspace past its deletion window have no rule of an owner left to keep, so
// internal/usecase/workspacedeletion deletes them directly. It may only remove, and only through the
// scoped wrappers, which exist for the Workspace-owned entities alone: a method of a wrapper that
// starts a scoped delete (Delete, Purge and their OneID forms), or a generic function instantiated
// with a scoped delete builder.
//
// An entry point is a driver of the statement of the tiers (tools/analyzers/tier): a handler of any
// surface, an MCP tool, a job or an event subscriber, a command. It receives input and calls a domain
// module or a use case; it owns no table, so every write there is a finding, worded for the entry
// point. The HTTP handlers (tier.IsHandler) do not read through ent either: a call of a method whose
// role starts a read (Get, a Query method, the scoped client's Workspace) is a finding there, and the
// read goes into the module that owns the entity. The other entry points may
// read. Nor does such a handler open a transaction (a method of the events bus that hands a callback
// the transaction's client or a scope on it, the Within… methods): the module does. The wiring (the router in internal/server included) is no entry point: a write there is any
// other package's, refused unless the package owns the table.
package entrywrite

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/getsphericon/sphericon/tools/analyzers/analysisx"
	"github.com/getsphericon/sphericon/tools/analyzers/entx"
	"github.com/getsphericon/sphericon/tools/analyzers/rule"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// eventsPath is the package of the events bus, whose Within… methods open a transaction (opensTx).
const eventsPath = tier.Module + "internal/infra/events"

// automationsPath is the package of the automations module, which owns several tables.
const automationsPath = "internal/domain/orchestration/automations"

// owners names, for each ent entity, the package that owns its table: the only one that writes it
// (ADR 0102). A domain module owns the tables of its domain; the infrastructure owns the few that
// belong to no domain (the Events of the bus, the Login throttle's attempts, the schema state the
// migrations keep); a hook package owns the log it keeps; the Enterprise Edition owns its own.
var owners = map[string]string{
	"ApiToken":                 "internal/domain/identity/apitokens",
	"AuditEntry":               "ee/domain/insights/auditentries",
	"AuthAttempt":              "internal/infra/throttle",
	"Automation":               automationsPath,
	"AutomationFigure":         automationsPath,
	"AutomationRun":            automationsPath,
	"AutomationStepRecord":     automationsPath,
	"AutomationVersion":        automationsPath,
	"Binding":                  "internal/domain/sending/bindings",
	"BrandKit":                 "internal/domain/content/brandkits",
	"Broadcast":                "internal/domain/orchestration/broadcasts",
	"BroadcastFigure":          "internal/domain/orchestration/broadcasts",
	"BroadcastRecipient":       "internal/domain/orchestration/broadcasts",
	"CapturedMessage":          "internal/domain/sending/capturedmessages",
	"Confirmation":             "internal/domain/contacts/confirmations",
	"ConsentRecord":            "internal/domain/identity/consentrecords",
	"ConsentStamp":             "internal/domain/contacts/consentstamps",
	"Contact":                  "internal/domain/contacts/contacts",
	"ContactTag":               "internal/domain/contacts/contacts",
	"CustomField":              "internal/domain/contacts/contacts",
	"DefaultSendCap":           "internal/domain/identity/suspension",
	"EmailTemplate":            "internal/domain/content/emailtemplates",
	"Erasure":                  "ee/domain/insights/erasures",
	"Event":                    "internal/infra/events",
	"EventJournalEntry":        "internal/infra/events",
	"EventReceipt":             "internal/infra/events",
	"HeldOrderEvent":           "internal/domain/contacts/orders",
	"Hold":                     "internal/domain/sending/holds",
	"IdentitySecret":           "internal/domain/contacts/identitytokens",
	"InstanceAuditEntry":       "internal/domain/instance/instanceauditentries",
	"InstanceState":            "internal/domain/instance/instancestates",
	"Integration":              "internal/domain/sending/integrations",
	"IntegrationProviderState": "internal/domain/sending/integrations",
	"Invitation":               "internal/domain/identity/invitations",
	"JoinDomain":               "internal/domain/identity/joindomains",
	"LegalDocumentDraft":       "internal/domain/identity/signupitems",
	"LegalDocumentVersion":     "internal/domain/identity/signupitems",
	"Membership":               "internal/domain/identity/memberships",
	"OAuthClient":              "internal/domain/identity/oauthclients",
	"OAuthCode":                "internal/domain/identity/oauthclients",
	"OAuthRefreshToken":        "internal/domain/identity/apitokens",
	"Order":                    "internal/domain/contacts/orders",
	"OrderMovement":            "internal/domain/contacts/orders",
	"OutboundMessage":          "internal/domain/sending/outboundmessages",
	"Passkey":                  "internal/domain/identity/passkeys",
	"PasskeyChallenge":         "internal/domain/identity/passkeys",
	"PendingSignup":            "internal/domain/identity/users",
	"PrivateRelayRange":        "internal/domain/sending/privaterelayranges",
	"ProviderLimiter":          "internal/domain/sending/providerlimit",
	"RecoveryCode":             "internal/domain/identity/secondfactors",
	"Rule":                     "internal/domain/orchestration/rules",
	"RuleFiring":               "internal/domain/orchestration/rules",
	"RuleVersion":              "internal/domain/orchestration/rules",
	"RuleVersionTrigger":       "internal/domain/orchestration/rules",
	"SchemaState":              "internal/infra/db",
	"SecondFactor":             "internal/domain/identity/secondfactors",
	"Segment":                  "internal/domain/contacts/segments",
	"SendCapLimiter":           "internal/domain/sending/providerlimit",
	"SendingDomain":            "internal/domain/sending/sendingdomains",
	"ServiceCredential":        "ee/domain/identity/servicecredentials",
	"SignupItem":               "internal/domain/identity/signupitems",
	"SignupSurveyResponse":     "internal/domain/identity/signupsurveyresponses",
	"StaffMember":              "internal/domain/identity/staffmembers",
	"StartToken":               "internal/domain/sending/bindings",
	"Suppression":              "internal/domain/contacts/suppressions",
	"Tag":                      "internal/domain/contacts/tags",
	"TrackedLink":              "internal/domain/sending/outboundmessages",
	"TrackingDomain":           "internal/domain/sending/trackingdomains",
	"TrackingHostChange":       "internal/domain/sending/trackinghostchanges",
	"Unsubscribe":              "internal/domain/contacts/unsubscribes",
	"Usage":                    "ee/domain/insights/usages",
	"User":                     "internal/domain/identity/users",
	"UserIdentity":             "internal/domain/identity/users",
	"UserTombstone":            "internal/domain/identity/usertombstones",
	"Visitor":                  "internal/domain/contacts/visitors",
	"WebhookEndpoint":          "internal/domain/orchestration/webhookendpoints",
	"WebhookEndpointHealth":    "internal/domain/orchestration/webhookendpoints",
	"Workspace":                "internal/domain/identity/workspaces",
	"WorkspaceStanding":        "internal/domain/identity/workspaces",
}

// Owners returns a copy of the ownership map: each ent entity and the package that owns its table.
func Owners() map[string]string { return maps.Clone(owners) }

// eraser is the Workspace erase, which removes a Workspace-owned table's rows whoever owns it (ADR 0110).
const eraser = "internal/usecase/workspacedeletion"

// Analyzer reports ent writes outside the owner of the table and in entry-point packages.
var Analyzer = &analysis.Analyzer{
	Name:     "entrywrite",
	Doc:      "refuses a write to an ent entity outside the package that owns its table, and any write through ent in handlers, MCP tools, jobs and event subscribers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// checker holds what a package's findings depend on: its path, whether it is an entry point and
// whether it is an API surface's handlers.
type checker struct {
	pass  *analysis.Pass
	path  string
	entry bool
	reads bool
	erase bool
}

func run(pass *analysis.Pass) (any, error) {
	path := pass.Pkg.Path()
	if !strings.HasPrefix(path, tier.Module) || tier.IsTestOrGenerated(path) {
		return nil, nil
	}
	c := checker{pass: pass, path: path, entry: tier.IsDriver(path), reads: tier.IsHandler(path), erase: tier.In(path, eraser)}
	insp, err := analysisx.Inspector(pass)
	if err != nil {
		return nil, err
	}
	for call := range inspector.All[*ast.CallExpr](insp) {
		c.call(call)
	}
	// The instantiations are few, and the type checker lists them: no walk visits every identifier.
	instances := slices.SortedFunc(maps.Keys(pass.TypesInfo.Instances), func(a, b *ast.Ident) int { return cmp.Compare(a.Pos(), b.Pos()) })
	for _, id := range instances {
		c.instance(id, pass.TypesInfo.Instances[id])
	}
	return nil, nil
}

func (c checker) call(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if c.reads && opensTx(c.pass.TypesInfo, sel) {
		c.report(call.Pos(),
			"a handler opens a transaction ("+sel.Sel.Name+" on the events bus) and holds logic a domain module owns",
			"move the transaction and what runs in it into the domain module and call that module from here")
		return
	}
	fn, named, ok := entx.Method(c.pass.TypesInfo, sel)
	if !ok {
		return
	}
	role := entx.RoleOf(named, sel.Sel.Name)
	if c.reads && role == entx.Read {
		what := entx.EntityOf(named)
		if what == "" {
			what = named.Obj().Name()
		}
		c.report(call.Pos(),
			"a handler reads through ent ("+sel.Sel.Name+" on "+what+") instead of asking a domain module",
			"give the domain module that owns the read a function for it (a Get or the operation's own read) and call that from here")
		return
	}
	if role != entx.Start || chainStartsWrite(c.pass.TypesInfo, sel.X) {
		return
	}
	if c.erase && entx.KindOf(named) == entx.Wrapper && removes(fn) {
		return
	}
	entity := entx.EntityOf(named)
	c.write(call.Pos(), sel.Sel.Name+" on "+entity, entity)
}

// instance reads an instantiation of a generic function or type: a type argument that is an ent
// write builder makes it a write of that builder's entity.
func (c checker) instance(id *ast.Ident, inst types.Instance) {
	for t := range inst.TypeArgs.Types() {
		named, ok := entx.Named(t)
		if !ok {
			continue
		}
		kind := entx.KindOf(named)
		if !kind.Builds() {
			continue
		}
		if c.erase && kind == entx.ScopedDelete {
			return
		}
		c.write(id.Pos(), id.Name+" with "+named.Obj().Name(), entx.EntityOf(named))
		return
	}
}

// write reports a write to entity, worded by what is: how it writes, made by the package checked.
func (c checker) write(pos token.Pos, what, entity string) {
	if c.entry {
		c.report(pos,
			"an entry point writes through ent ("+what+") and holds logic a domain module owns",
			"move the write into the domain module that owns "+entity+" and call that module from here")
		return
	}
	owner, ok := owners[entity]
	if !ok {
		c.report(pos,
			"a write to "+entity+" ("+what+"), an entity the ownership map names no owner of",
			"add "+entity+" to owners in tools/analyzers/entrywrite with the domain module that owns its table")
		return
	}
	if tier.In(c.path, owner) {
		return
	}
	pkg := strings.TrimPrefix(c.path, tier.Module)
	c.report(pos,
		pkg+" writes "+entity+" ("+what+"), a table "+owner+" owns",
		"give "+owner+" a function that makes the write and call it from here (ADR 0102: only the owner writes a table)")
}

// report reports a finding of rule 5, unless it stands in a test file, which the analyzer does not
// read. The file is asked last, of a finding only: most nodes are dismissed by their types first.
func (c checker) report(pos token.Pos, what, fix string) {
	if analysisx.InTestFile(c.pass, pos) {
		return
	}
	rule.Report(c.pass, pos, 5, what, fix)
}

// opensTx reports whether the selector names a method of the events bus that opens a transaction:
// one that takes a callback handed a type of the ent package (the transaction's client, a scope on
// it or the Workspace lock's scope), whatever the method is called.
func opensTx(info *types.Info, sel *ast.SelectorExpr) bool {
	fn, ok := info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Signature().Recv() == nil || fn.Pkg() == nil || fn.Pkg().Path() != eventsPath {
		return false
	}
	for p := range fn.Signature().Params().Variables() {
		callback, ok := p.Type().Underlying().(*types.Signature)
		if !ok {
			continue
		}
		for q := range callback.Params().Variables() {
			if _, ok := entx.Named(q.Type()); ok {
				return true
			}
		}
	}
	return false
}

// removes reports whether a method returns a scoped delete builder: the Delete or Purge of a wrapper.
func removes(fn *types.Func) bool {
	res := fn.Signature().Results()
	if res.Len() == 0 {
		return false
	}
	named, ok := entx.Named(res.At(0).Type())
	return ok && entx.KindOf(named) == entx.ScopedDelete
}

// chainStartsWrite reports whether the receiver chain already holds a call that starts a write, so
// that `Create().OnConflict...` and `Pointers().CompareAndSwap(...)` are one finding each, and an upsert on
// a builder variable is another.
func chainStartsWrite(info *types.Info, e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if _, named, ok := entx.Method(info, sel); ok && entx.RoleOf(named, sel.Sel.Name) == entx.Start {
			return true
		}
		e = sel.X
	}
}
