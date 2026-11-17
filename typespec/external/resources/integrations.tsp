using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

// --- Output config (secrets redacted; never echo passwords/keys) ---

// The input config (carries secrets; write-only) is common/integrations.tsp.

// --- Resource + request bodies ---

/** A sending-provider Integration. Credentials are write-only: the config is returned with secrets redacted. */
@resource("integrations")
@segment("integrations")
model IntegrationResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Human-readable label */
  @untrusted
  name: string;

  /** Delivery channel */
  channel: IntegrationChannel;

  /** Concrete provider */
  provider: IntegrationProvider;

  /** Whether this integration may be used for sending */
  enabled: boolean;

  /** Whether this is the default provider for its channel */
  isDefault: boolean;

  /** Provider limit set on the Integration: most messages per second; null means none */
  maxPerSecond: int32 | null;

  /** Provider limit set on the Integration: most messages per rolling 24 hours; null means none */
  maxPerDay: int32 | null;

  /** The Provider limit as enforced and 24-hour usage */
  providerLimit: ProviderLimitStatus;

  /** Provider config (secrets redacted) */
  config: IntegrationConfig;

  /** The Preset this Integration's provider callbacks are read with; null when it receives none */
  preset: string | null;

  /** The Preset's params as saved. A secret param (`writeOnly` in the Preset's schema) is shown as `********`, never its value */
  params: JsonObject;

  /** Names of the secret params that are rotating: a retiring value is still accepted beside the current one */
  secretsRetiring: string[];

  /** Where the provider posts this Integration's callbacks: paste it into the provider. The key in it is an address, not the authentication (the Preset's signature or secret is) */
  hookUrl: string;

  /** When the integration was deleted (discarded); null while it is kept. A deleted integration is no longer listed, is read-only and cannot be restored, but stays readable by id for the history that names it; it is never the default, takes no new work, and its hook still reads the provider's callbacks. */
  discardedAt?: Timestamp | null;

  /** The revision of the state this answer shows (ADR 0119): an update names it. A provider's reading of the integration (its quota, standing and refusal) does not move it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/**
 * A deleted integration is discarded, not removed physically (ADR 0049): its sent mail keeps the
 * channel it went through. Its delete answers 409 while live work uses it.
 */
@tag("Integrations")
interface Integrations
  extends ExternalCrud<
      IntegrationResource,
      CreateIntegrationInput,
      UpdateIntegrationInput,
      ApiTokenScope.integrationsRead,
      ApiTokenScope.integrationsWrite,
      ApiTokenScope.integrationsWrite
    > {
  /** Replace the Integration's hook key: the old hook URL stops working at once and the new one must be pasted into the provider; an enabled Telegram integration's webhook follows it (Telegram's refusal is a 422); a deleted integration answers 404. Needs a person: hidden from MCP. */
  @action("regenerate-key")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  regenerateKey(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | Problem;
}

@@doc(Integrations.list, "List integrations with pagination");
@@doc(Integrations.create, "Create an integration");
@@doc(Integrations.get, "Get an integration by ID, deleted or not");
@@doc(
  Integrations.update,
  "Update an integration; a deleted one is read-only and answers 404"
);
@@doc(
  Integrations.delete,
  "Delete an integration for good: it leaves the list, gives up the default and is never restored; what it sent keeps its history. Refused with 409 while a scheduled or sending broadcast, or an active email automation through the default, uses it (`usedBy` names them). A Telegram integration's webhook is removed, best effort."
);

// --- Presets (ADR 0037) ---
//
// Exception to "type fixed structures in the contract": a Preset's params are dynamic by
// nature (a Postmark token, a Mailgun signing key, an SNS topic), so the contract carries
// the Preset's JSON Schema (`paramsSchema`) and an Integration's values (`params`) as
// opaque objects. They are validated server side against that schema, never by the
// generated client: read `paramsSchema` from `presets_list` before sending `params`.

@route("/presets")
@tag("Integrations")
namespace Presets {
  /** List the Presets the Instance offers, with the params schema of each */
  @get
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  op list(): Page<PresetResource> | Problem;
}
