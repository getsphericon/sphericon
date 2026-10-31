// `@everyOperationDeclares("scopes")`, as /api declares it, and @requiresScopes.
import "@typespec/http";
import "../../common/scopes.tsp";

using TypeSpec.Http;
using Sphericon;

@service(#{ title: "Fixture" })
@everyOperationDeclares("scopes")
namespace Fixture;

@route("/contacts")
@get
@requiresScopes(#[ApiTokenScope.contactsRead])
op contacts(): void;

@route("/me")
@get
@requiresScopes(#[])
op me(): void;

// expect-next-line: sphericon/scopes-missing
@route("/tags")
@get
op tags(): void;

/** A template's operation takes its scope from the template, as the CRUD templates do */
interface Reads<Scope extends valueof ApiTokenScope> {
  @get
  @requiresScopes(#[Scope])
  list(): void;
}

@route("/events")
interface Events extends Reads<ApiTokenScope.eventsRead> {}
