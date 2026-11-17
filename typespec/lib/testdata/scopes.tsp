// `@everyOperationDeclares("scopes", "problem")`, as /api declares it, and @requiresScopes: each
// rule fails on its own.
import "@typespec/http";
import "../../common/scopes.tsp";
import "../../common/scalars.tsp";
import "../../common/errors.tsp";

using TypeSpec.Http;
using Sphericon;

@service(#{ title: "Fixture" })
@everyOperationDeclares("scopes", "problem")
namespace Fixture;

@route("/contacts")
@get
@requiresScopes(#[ApiTokenScope.contactsRead])
op contacts(): void | Problem;

@route("/me")
@get
@requiresScopes(#[])
op me(): void | Problem;

// expect-next-line: sphericon/scopes-missing
@route("/tags")
@get
op tags(): void | Problem;

/** A template's operation takes its scope from the template, as the CRUD templates do */
interface Reads<Scope extends valueof ApiTokenScope> {
  @get
  @requiresScopes(#[Scope])
  list(): void | Problem;
}

@route("/events")
interface Events extends Reads<ApiTokenScope.eventsRead> {}

// expect-next-line: sphericon/problem-missing
@route("/segments")
@get
@requiresScopes(#[ApiTokenScope.contactsRead])
op segments(): void;
