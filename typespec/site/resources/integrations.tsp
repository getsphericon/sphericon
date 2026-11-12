using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** Delivery channel an integration belongs to */
enum SiteIntegrationChannel {
  email,
  sms,

  /** A Workspace's own Telegram bot */
  telegram,
}

/** Concrete sending provider */
enum SiteIntegrationProvider {
  smtp,
  ses,

  /** The Test inbox: sends nothing outside and keeps each message as a Captured message */
  inbox,

  /** A Telegram bot, through the Bot API */
  telegram,

  /** A Twilio account, on the `sms` channel */
  twilio,
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
  /** No ceiling applies: sends are not paced and the provider may throttle or block them (never set on a Test inbox, which sends to no provider) */
  unlimited,

  /** The provider's send quota could not be read (no permission, or a compatible service without the call), so only the manual limit applies */
  providerQuotaUnavailable,

  /** The provider has stopped the account (SES enforcement SHUTDOWN): sends through the Integration are held until it sends again */
  providerStopped,

  /** The provider has put the account under review (SES enforcement PROBATION); it still sends */
  providerProbation,

  /** The account is in the provider's sandbox (SES without production access): it delivers only to verified addresses */
  providerSandbox,
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

/** A Telegram bot without its token: who it is, as Telegram said when it was connected */
model SiteTelegramConfig {
  kind: "telegram";

  /** The bot's numeric Telegram id */
  botId: int64;

  /** The bot's @username, without the @ */
  username: string;
}

/** A Twilio account without its secret */
model SiteTwilioConfig {
  kind: "twilio";

  /** The Account SID */
  accountSid: string;

  /** The API key messages are sent with; null when they are sent with the Auth Token */
  apiKeySid: string | null;

  /** What messages go out from */
  from: string;

  /** What kind of sender `from` is */
  fromKind: SmsFromKind;

  /** Whether connecting pointed the sender's callbacks at the hook. False for an alphanumeric sender ID, which receives nothing, and when the credentials may not change the sender: then paste `hookUrl` into Twilio as the sender's inbound and status callback */
  callbacksSet: boolean;
}

/** Provider config returned to the UI, discriminated by `kind`. Secrets are omitted. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union SiteIntegrationConfig {
  smtp: SiteSmtpConfig,
  ses: SiteSesConfig,
  inbox: SiteInboxConfig,
  telegram: SiteTelegramConfig,
  twilio: SiteTwilioConfig,
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

/** A Preset the Instance offers */
model SitePresetResource {
  /** The name an integration's `preset` refers to */
  name: string;

  /** What the Preset is for */
  description: string;

  /** JSON Schema (draft 2020-12) of the params; a string marked `writeOnly` is a secret */
  paramsSchema: JsonObject;
}

@route("/presets")
@tag("Site Integrations")
namespace SitePresets {
  /** List the Presets the Instance offers, with the params schema of each */
  @get
  op list(): SitePresetResource[] | Problem;
}
