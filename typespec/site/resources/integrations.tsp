using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

// --- Output config (secrets redacted; never echo passwords/keys) ---

// The input config (carries secrets; write-only) is common/integrations.tsp.

// --- Resource + request bodies ---

/** A workspace sending-provider integration */
@resource("integrations")
@segment("integrations")
@parentResource(SiteWorkspaceResource)
model SiteIntegrationResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Human-readable label */
  name: string;

  /** Delivery channel */
  channel: IntegrationChannel;

  /** Concrete provider */
  provider: IntegrationProvider;

  /** Whether this integration may be used for sending */
  enabled: boolean;

  /** Whether this is the default provider for its channel */
  isDefault: boolean;

  /** Provider limit: most messages per second; null means no per-second limit */
  maxPerSecond: int32 | null;

  /** Provider limit: most messages per rolling 24 hours; null means no daily limit */
  maxPerDay: int32 | null;

  /** The Provider limit as enforced (manual and provider-reported values) and 24-hour usage */
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

  /** When the integration was deleted (discarded); null while it is kept. A deleted integration is no longer listed, is read-only and cannot be restored, but stays readable by id for the history that names it; it is never the default, takes no new work, and its hook still reads the provider's callbacks (a Telegram bot's webhook is removed, so Telegram sends it none). */
  discardedAt?: Timestamp | null;

  /** The revision of the state this answer shows (ADR 0119): an update names it. A provider's reading of the integration (its quota, standing and refusal) does not move it */
  revision: Revision;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

// Bespoke (not WorkspaceCrud): `list` returns a bare array, not a paginated
// envelope. Nesting under /workspaces/{workspaceId} comes from @parentResource + @autoRoute.
@tag("Site Integrations")
namespace SiteIntegrations {
  /** List the workspace's sending-provider integrations; a deleted one is not listed */
  @listsResource(SiteIntegrationResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op list(...ResourceCollectionParameters<SiteIntegrationResource>):
    | {
        @statusCode statusCode: 200;
        @body body: SiteIntegrationResource[];
      }
    | Problem;

  /** Create an integration. Owners and admins only. */
  @createsResource(SiteIntegrationResource)
  @post
  @autoRoute
  @requiresPermissions(#[Permission.integrationsManage])
  op create(
    ...ResourceCollectionParameters<SiteIntegrationResource>,
    @body body: CreateIntegrationInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteIntegrationResource;
      }
    | Problem;

  /** Get an integration by id, deleted or not */
  @readsResource(SiteIntegrationResource)
  @get
  @autoRoute
  @requiresPermissions(#[])
  op get(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | Problem;

  /** Update an integration; a deleted one is read-only and answers 404. Owners and admins only. */
  @updatesResource(SiteIntegrationResource)
  @put
  @autoRoute
  @requiresPermissions(#[Permission.integrationsManage])
  op update(
    ...ResourceParameters<SiteIntegrationResource>,
    @body body: UpdateIntegrationInput,
  ): SiteIntegrationResource | Problem;

  /** Replace the integration's hook key: the old hook URL stops working at once; an enabled Telegram integration's webhook follows it (Telegram's refusal is a 422); a deleted integration answers 404. Owners and admins only. */
  @action("regenerate-key")
  @post
  @autoRoute
  @requiresPermissions(#[Permission.integrationsManage])
  op regenerateKey(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | Problem;

  /** Delete an integration for good: it leaves the list, gives up the default and is never restored; what it sent keeps its history. Refused with 409 while a scheduled or sending broadcast, or an active email automation through the default, uses it. A Telegram integration's webhook is removed, best effort. Owners and admins only. */
  @deletesResource(SiteIntegrationResource)
  @delete
  @autoRoute
  @requiresPermissions(#[Permission.integrationsManage])
  op delete(...ResourceParameters<SiteIntegrationResource>):
    | NoContentResponse
    | Problem;
}

// A Preset's params are dynamic by nature, so the contract carries the Preset's JSON Schema
// and an Integration's values as opaque objects, validated server side (ADR 0037): the
// recorded exception to "type fixed structures in the contract". ADR 0066 asks the app to
// edit the params in a form generated from that schema.

@route("/presets")
@tag("Site Integrations")
namespace SitePresets {
  /** List the Presets the Instance offers, with the params schema of each */
  @get
  op list(): PresetResource[] | Problem;
}
