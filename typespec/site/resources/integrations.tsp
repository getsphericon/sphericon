using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Delivery channel an integration belongs to */
enum SiteIntegrationChannel {
  email,
  sms,
}

/** Concrete sending provider */
enum SiteIntegrationProvider {
  smtp,
  ses,

  /** The Test inbox: sends nothing outside and keeps each message as a Captured message */
  inbox,
}

/** Where an effective Provider limit comes from */
enum SiteProviderLimitSource {
  /** Set by the user on the Integration */
  manual,

  /** Reported by the provider */
  provider,
}

/** A reason to look at an Integration's Provider limit */
enum SiteProviderLimitWarning {
  /** No ceiling applies: sends are not paced and the provider may throttle or block them */
  unlimited,

  /** The provider's send quota could not be read (no permission, or a compatible service without the call), so only the manual limit applies */
  providerQuotaUnavailable,
}

/** One effective ceiling: the lowest of the manual and provider values, manual on a tie */
model SiteProviderLimitValue {
  /** The ceiling in messages; null means this window is not limited */
  limit: int32 | null;

  /** Where the ceiling comes from; null when there is none */
  source: SiteProviderLimitSource | null;
}

/** The Provider limit as enforced, with the last 24 hours of usage */
model SiteProviderLimitStatus {
  /** Most messages per second */
  perSecond: SiteProviderLimitValue;

  /** Most messages per rolling 24 hours */
  perDay: SiteProviderLimitValue;

  /** Messages the provider accepted from this Integration in the last 24 hours */
  sentLast24h: int32;

  /** Reasons to review the limit; empty when there is nothing to flag */
  warnings: SiteProviderLimitWarning[];
}

// --- Output config (secrets redacted; never echo passwords/keys) ---

/** SMTP config without the password */
model SiteSmtpConfig {
  kind: "smtp";
  host: string;
  port: int32;
  username?: string | null;
  from: EmailAddress;
  fromName?: string | null;
}

/** SES config without the secret access key */
model SiteSesConfig {
  kind: "ses";
  region: string;
  from: EmailAddress;
  fromName?: string | null;

  /** Custom SES-compatible API endpoint (e.g. Yandex Cloud Postbox); empty for AWS */
  endpoint?: string | null;

  /** Last 4 chars of the access key id, for recognition */
  accessKeyIdLast4?: string | null;
}

/** The Test inbox has no settings */
model SiteInboxConfig {
  kind: "inbox";
}

/** Provider config returned to the UI, discriminated by `kind`. Secrets are omitted. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteIntegrationConfig {
  smtp: SiteSmtpConfig,
  ses: SiteSesConfig,
  inbox: SiteInboxConfig,
}

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
  channel: SiteIntegrationChannel;

  /** Concrete provider */
  provider: SiteIntegrationProvider;

  /** Whether this integration may be used for sending */
  enabled: boolean;

  /** Whether this is the default provider for its channel */
  isDefault: boolean;

  /** Provider limit: most messages per second; null means no per-second limit */
  maxPerSecond: int32 | null;

  /** Provider limit: most messages per rolling 24 hours; null means no daily limit */
  maxPerDay: int32 | null;

  /** The Provider limit as enforced (manual and provider-reported values) and 24-hour usage */
  providerLimit: SiteProviderLimitStatus;

  /** Provider config (secrets redacted) */
  config: SiteIntegrationConfig;

  /** The Preset this Integration's provider callbacks are read with; null when it receives none */
  preset: string | null;

  /** The Preset's params as saved. A secret param (`writeOnly` in the Preset's schema) is shown as `********`, never its value */
  params: Record<unknown>;

  /** Names of the secret params that are rotating: a retiring value is still accepted beside the current one */
  secretsRetiring: string[];

  /** Where the provider posts this Integration's callbacks: paste it into the provider. The key in it is an address, not the authentication (the Preset's signature or secret is) */
  hookUrl: string;

  /** When the integration was archived; null while it is kept. An archived integration is never the default and takes no new work, but its hook still reads the provider's callbacks. */
  archivedAt?: Timestamp | null;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Filter of the integration list */
model SiteIntegrationsListFilter {
  /** true lists the archived integrations instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

// Bespoke (not WorkspaceCrud): `list` returns a bare array, not a paginated
// envelope. Nesting under /workspaces/{workspaceId} comes from @parentResource + @autoRoute.
@tag("Site Integrations")
namespace SiteIntegrations {
  /** List the workspace's sending-provider integrations: the kept ones, or with `archived` the archived ones */
  @listsResource(SiteIntegrationResource)
  @get
  @autoRoute
  @extension("x-required-permissions", #[])
  op list(
    ...ResourceCollectionParameters<SiteIntegrationResource>,
    ...SiteIntegrationsListFilter,
  ):
    | {
        @statusCode statusCode: 200;
        @body body: SiteIntegrationResource[];
      }
    | NotFoundProblem;

  /** Create an integration. Owners and admins only. */
  @createsResource(SiteIntegrationResource)
  @post
  @autoRoute
  @extension("x-required-permissions", #[Permission.integrationsManage])
  op create(
    ...ResourceCollectionParameters<SiteIntegrationResource>,
    @body body: CreateIntegrationInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: SiteIntegrationResource;
      }
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Get an integration by id */
  @readsResource(SiteIntegrationResource)
  @get
  @autoRoute
  @extension("x-required-permissions", #[])
  op get(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Update an integration. Owners and admins only. */
  @updatesResource(SiteIntegrationResource)
  @put
  @autoRoute
  @extension("x-required-permissions", #[Permission.integrationsManage])
  op update(
    ...ResourceParameters<SiteIntegrationResource>,
    @body body: UpdateIntegrationInput,
  ):
    | SiteIntegrationResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Replace the integration's hook key: the old hook URL stops working at once. Owners and admins only. */
  @action("regenerate-key")
  @post
  @autoRoute
  @extension("x-required-permissions", #[Permission.integrationsManage])
  op regenerateKey(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Archive an integration: it leaves the list and gives up the default, and can be restored (it does not become the default again). Refused with 409 while a scheduled or sending broadcast, or an active email automation through the default, uses it. Owners and admins only. */
  @action("archive")
  @post
  @autoRoute
  @extension("x-required-permissions", #[Permission.integrationsManage])
  op archive(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived integration to the list. It is not the default; promote it with an update. Refused with 409 when it is a Test inbox and the Workspace keeps another. Owners and admins only. */
  @action("unarchive")
  @post
  @autoRoute
  @extension("x-required-permissions", #[Permission.integrationsManage])
  op unarchive(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem
    | ConflictProblem;
}

// A Preset's params are dynamic by nature, so the contract carries the Preset's JSON Schema
// and an Integration's values as opaque objects, validated server side (ADR 0037): the
// recorded exception to "type fixed structures in the contract". ADR 0066 asks the app to
// edit the params in a form generated from that schema.

/** A Preset the Instance offers */
model SitePresetResource {
  /** The name an integration's `preset` refers to */
  name: string;

  /** What the Preset is for */
  description: string;

  /** JSON Schema (draft 2020-12) of the params; a string marked `writeOnly` is a secret */
  paramsSchema: Record<unknown>;
}

@route("/presets")
@tag("Site Integrations")
namespace SitePresets {
  /** List the Presets the Instance offers, with the params schema of each */
  @get
  op list(): SitePresetResource[];
}
