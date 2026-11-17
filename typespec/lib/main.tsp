// The contract decorators that name no enum of the contract (docs/agents/codegen.md, "Contract
// decorators"). Each writes the `x-*` extension the Go readers of the OpenAPI documents read;
// decorators.ts implements and checks them. The ones typed by a contract enum are declared beside
// it: @requiresPermissions in common/permissions.tsp, @requiresScopes, @grantable and @grantedWith
// in common/scopes.tsp, @requiresServiceScopes in service/resources/identity.tsp.
import "./decorators.ts";

namespace Sphericon;

/**
 * How an /api operation is projected as an MCP tool (ADR 0016), as `x-mcp`. Every operation is a
 * tool named by its operation id in snake_case unless it is `hidden`; `name` overrides the name;
 * `send` makes it a send-class tool, listed and callable only with the `mcp:send` scope (ADR 0065).
 */
extern dec mcp(
  target: Reflection.Operation,
  tool: valueof {
    hidden?: boolean,
    name?: string,
    send?: boolean,
  }
);

/** The property carries text a caller wrote, which an MCP client must treat as data (ADR 0016), as `x-untrusted` */
extern dec untrusted(target: Reflection.ModelProperty);

/** A User withheld by the Two-factor requirement may still call the operation in the Workspace (ADR 0020), as `x-allowed-while-withheld` */
extern dec allowedWhileWithheld(target: Reflection.Operation);

/** A User with a required acceptance pending may still call the operation (ADR 0062), as `x-allowed-while-acceptance-pending` */
extern dec allowedWhileAcceptancePending(target: Reflection.Operation);

/**
 * The server limits the operation per client address under the policy, as `x-rate-limit`: `login`,
 * one budget every step of a login shares; `forgot_password`, the reset requests'; `human`, a budget
 * of its own for each public form a person submits
 */
extern dec rateLimited(
  target: Reflection.Operation,
  policy: valueof "login" | "forgot_password" | "human"
);

/**
 * What every operation of the surface declares, checked at compile, one or more rules. `scopes`:
 * its scopes, `#[]` for none. `workspacePermissions`: an operation with `workspaceId` in its path
 * declares its Permissions, `#[]` for any member, and no other operation declares Permissions or
 * `@allowedWhileWithheld`. `problem`: it returns its success type `| Problem`, the one `default`
 * response every refusal and failure is answered with (ADR 0103).
 */
extern dec everyOperationDeclares(
  target: Reflection.Namespace,
  ...rules: valueof ("scopes" | "workspacePermissions" | "problem")[]
);

/**
 * Every request body of the surface is a closed object at every depth (ADR 0148): a property the
 * contract does not declare is refused. A map in a body must be declared `@openMap`.
 */
extern dec closedRequestBodies(target: Reflection.Namespace);

/**
 * An object whose keys the contract does not own (ADR 0148): a map whose keys are data, such as
 * Custom field values or an Event's properties (ADR 0006), or a document another schema checks,
 * such as the email editor's or a WebAuthn response. A request body may carry any key in it.
 */
extern dec openMap(target: Reflection.Model);
