using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;

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
}

/** A per-second Send rate limit the user may set */
@minValue(1)
@maxValue(10000)
scalar SiteMaxPerSecond extends int32;

/** A rolling 24-hour Send rate limit the user may set */
@minValue(1)
@maxValue(100000000)
scalar SiteMaxPerDay extends int32;

/** Where an effective Send rate limit comes from */
enum SiteSendLimitSource {
  /** Set by the user on the Integration */
  manual,

  /** Reported by the provider */
  provider,
}

/** A reason to look at an Integration's Send rate limit */
enum SiteSendLimitWarning {
  /** No ceiling applies: sends are not paced and the provider may throttle or block them */
  unlimited,

  /** The provider's send quota could not be read (no permission, or a compatible service without the call), so only the manual limit applies */
  providerQuotaUnavailable,
}

/** One effective ceiling: the lowest of the manual and provider values, manual on a tie */
model SiteSendLimitValue {
  /** The ceiling in messages; null means this window is not limited */
  limit: int32 | null;

  /** Where the ceiling comes from; null when there is none */
  source: SiteSendLimitSource | null;
}

/** The Send rate limit as enforced, with the last 24 hours of usage */
model SiteSendLimitStatus {
  /** Most messages per second */
  perSecond: SiteSendLimitValue;

  /** Most messages per rolling 24 hours */
  perDay: SiteSendLimitValue;

  /** Messages the provider accepted from this Integration in the last 24 hours */
  sentLast24h: int32;

  /** Reasons to review the limit; empty when there is nothing to flag */
  warnings: SiteSendLimitWarning[];
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

  /** The one SNS topic bounce and complaint notifications are accepted from; none means the hook refuses all */
  snsTopicArn?: string | null;

  /** Last 4 chars of the access key id, for recognition */
  accessKeyIdLast4?: string | null;
}

/** Provider config returned to the UI, discriminated by `kind`. Secrets are omitted. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteIntegrationConfig {
  smtp: SiteSmtpConfig,
  ses: SiteSesConfig,
}

// --- Input config (carries secrets; write-only) ---

/** SMTP credentials */
model SiteSmtpConfigInput {
  kind: "smtp";
  host: string;
  port: int32;
  username?: string | null;
  password?: string | null;
  from: EmailAddress;
  fromName?: string | null;
}

/** SES credentials */
model SiteSesConfigInput {
  kind: "ses";
  region: string;
  accessKeyId: string;
  secretAccessKey: string;
  from: EmailAddress;
  fromName?: string | null;

  /** Custom SES-compatible API endpoint (e.g. Yandex Cloud Postbox); empty for AWS */
  endpoint?: string | null;

  /** The SNS topic (ARN) this Integration's bounces and complaints arrive through; notifications from any other topic are refused */
  snsTopicArn?: string | null;
}

/** Provider config submitted by the UI, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteIntegrationConfigInput {
  smtp: SiteSmtpConfigInput,
  ses: SiteSesConfigInput,
}

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

  /** Send rate limit: most messages per second; null means no per-second limit */
  maxPerSecond: int32 | null;

  /** Send rate limit: most messages per rolling 24 hours; null means no daily limit */
  maxPerDay: int32 | null;

  /** The Send rate limit as enforced (manual and provider-reported values) and 24-hour usage */
  sendLimit: SiteSendLimitStatus;

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

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Create a workspace integration. The provider/channel are derived from `config.kind`. */
model SiteCreateIntegrationInput {
  name: string;
  enabled?: boolean;
  isDefault?: boolean;

  /** Most messages per second (1 to 10 000); omit or null for no limit */
  maxPerSecond?: SiteMaxPerSecond | null;

  /** Most messages per rolling 24 hours (1 to 100 000 000); omit or null for no limit */
  maxPerDay?: SiteMaxPerDay | null;

  config: SiteIntegrationConfigInput;

  /** Name of a Preset from `presets_list` to read provider callbacks (bounces, complaints) with */
  preset?: string | null;

  /** The Preset's params, validated against its `paramsSchema`: read the schema first */
  params?: Record<unknown>;
}

/** Update a workspace integration. Omit `config` to keep stored credentials. */
model SiteUpdateIntegrationInput {
  name?: string;
  enabled?: boolean;
  isDefault?: boolean;

  /** Most messages per second (1 to 10 000); null clears the limit, omit to keep */
  maxPerSecond?: SiteMaxPerSecond | null;

  /** Most messages per rolling 24 hours (1 to 100 000 000); null clears the limit, omit to keep */
  maxPerDay?: SiteMaxPerDay | null;

  config?: SiteIntegrationConfigInput | null;

  /** Name of a Preset from `presets_list`; null removes it. Changing it requires `params` in full: no secret carries over */
  preset?: string | null;

  /** The Preset's params. A secret that is blank, omitted or `********` stays as saved; a new value rotates it (the old one stays valid until retired) */
  params?: Record<unknown>;

  /** Stop accepting the retiring value of every rotating secret */
  retireSecrets?: boolean;
}

// Bespoke (not WorkspaceCrud): `list` returns a bare array, not a paginated
// envelope. Nesting under /workspaces/{slug} comes from @parentResource + @autoRoute.
@tag("Site Integrations")
namespace SiteIntegrations {
  /** List the workspace's sending-provider integrations */
  @listsResource(SiteIntegrationResource)
  @get
  @autoRoute
  op list(...ResourceCollectionParameters<SiteIntegrationResource>):
    | {
        @statusCode statusCode: 200;
        @body body: SiteIntegrationResource[];
      }
    | NotFoundProblem;

  /** Create an integration. Owners and admins only. */
  @createsResource(SiteIntegrationResource)
  @post
  @autoRoute
  op create(
    ...ResourceCollectionParameters<SiteIntegrationResource>,
    @body body: SiteCreateIntegrationInput,
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
  op get(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | BadRequestProblem
    | NotFoundProblem;

  /** Update an integration. Owners and admins only. */
  @updatesResource(SiteIntegrationResource)
  @put
  @autoRoute
  op update(
    ...ResourceParameters<SiteIntegrationResource>,
    @body body: SiteUpdateIntegrationInput,
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
  op regenerateKey(...ResourceParameters<SiteIntegrationResource>):
    | SiteIntegrationResource
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;

  /** Delete an integration. Owners and admins only. */
  @deletesResource(SiteIntegrationResource)
  @delete
  @autoRoute
  op delete(...ResourceParameters<SiteIntegrationResource>):
    | NoContentResponse
    | BadRequestProblem
    | ForbiddenProblem
    | NotFoundProblem;
}

// A Preset's params are dynamic by nature, so the contract carries the Preset's JSON Schema
// and an Integration's values as opaque objects, validated server side (ADR 0037): the
// recorded exception to "type fixed structures in the contract". The console offers a
// Preset select and a JSON editor for the params, never a form generated from the schema.

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
