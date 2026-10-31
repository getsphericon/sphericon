// The contract decorators that name no enum of the contract (docs/agents/codegen.md, "Contract
// decorators"). Each writes the `x-*` extension the Go readers of the OpenAPI documents read;
// decorators.js implements and checks them. The ones typed by a contract enum are declared beside
// it: @requiresPermissions in common/permissions.tsp, @requiresScopes and @grants in
// common/scopes.tsp, @requiresServiceScopes in service/resources/identity.tsp.
import "./decorators.js";

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
 * What every operation of the surface declares, checked at compile. `scopes`: its scopes, `#[]`
 * for none. `workspacePermissions`: an operation with `workspaceId` in its path declares its
 * Permissions, `#[]` for any member, and no other operation declares Permissions or
 * `@allowedWhileWithheld`.
 */
extern dec everyOperationDeclares(
  target: Reflection.Namespace,
  what: valueof "scopes" | "workspacePermissions"
);
