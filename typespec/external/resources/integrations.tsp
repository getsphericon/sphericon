using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
using TypeSpec.OpenAPI;

namespace Sphericon;

// --- Output config (secrets redacted; never echo passwords/keys) ---

/** SMTP config without the password */
model SmtpConfig {
  kind: "smtp";
  host: string;
  port: int32;
  username?: string | null;
  from: EmailAddress;
  fromName?: string | null;
}

/** SES config without the secret access key */
model SesConfig {
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
model InboxConfig {
  kind: "inbox";
}

/** A Telegram bot without its token: who it is, as Telegram said when it was connected */
model TelegramConfig {
  kind: "telegram";

  /** The bot's numeric Telegram id */
  botId: int64;

  /** The bot's @username, without the @ */
  username: string;
}

/** A Twilio account without its secret */
model TwilioConfig {
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

/** Provider config as returned, discriminated by `kind`. Secrets are omitted. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union IntegrationConfig {
  smtp: SmtpConfig,
  ses: SesConfig,
  inbox: InboxConfig,
  telegram: TelegramConfig,
  twilio: TwilioConfig,
}

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
 * channel it went through. Not built on ExternalCrud: its delete answers 409 while live work uses it.
 */
@tag("Integrations")
interface Integrations {
  /** List integrations with pagination */
  @listsResource(IntegrationResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  list(...ResourceCollectionParameters<IntegrationResource>, ...PageQuery):
    | Page<IntegrationResource>
    | Problem;

  /** Create an integration */
  @createsResource(IntegrationResource)
  @post
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  create(
    ...ResourceCollectionParameters<IntegrationResource>,
    @body body: CreateIntegrationInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: IntegrationResource;
      }
    | Problem;

  /** Get an integration by ID, deleted or not */
  @readsResource(IntegrationResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  get(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | Problem;

  /** Update an integration; a deleted one is read-only and answers 404 */
  @updatesResource(IntegrationResource)
  @put
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  update(
    ...ResourceParameters<IntegrationResource>,
    @body body: UpdateIntegrationInput,
  ): IntegrationResource | Problem;

  /** Delete an integration for good: it leaves the list, gives up the default and is never restored; what it sent keeps its history. Refused with 409 while a scheduled or sending broadcast, or an active email automation through the default, uses it (`usedBy` names them). A Telegram integration's webhook is removed, best effort. */
  @deletesResource(IntegrationResource)
  @delete
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  delete(...ResourceParameters<IntegrationResource>):
    | NoContentResponse
    | Problem;

  /** Replace the Integration's hook key: the old hook URL stops working at once and the new one must be pasted into the provider; an enabled Telegram integration's webhook follows it (Telegram's refusal is a 422); a deleted integration answers 404. Needs a person: hidden from MCP. */
  @action("regenerate-key")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  regenerateKey(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | Problem;
}

// --- Presets (ADR 0037) ---
//
// Exception to "type fixed structures in the contract": a Preset's params are dynamic by
// nature (a Postmark token, a Mailgun signing key, an SNS topic), so the contract carries
// the Preset's JSON Schema (`paramsSchema`) and an Integration's values (`params`) as
// opaque objects. They are validated server side against that schema, never by the
// generated client: read `paramsSchema` from `presets_list` before sending `params`.

/** A Preset: how an Instance verifies and reads one provider's bounce and complaint callbacks */
model PresetResource {
  /** The name an Integration's `preset` refers to */
  name: string;

  /** What the Preset is for */
  description: string;

  /** JSON Schema (draft 2020-12) of the `params` an Integration of this Preset takes. A string marked `writeOnly` is a secret: it is never returned by a read. */
  paramsSchema: JsonObject;
}

@route("/presets")
@tag("Integrations")
namespace Presets {
  /** List the Presets the Instance offers, with the params schema of each */
  @get
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  op list(): Page<PresetResource> | Problem;
}
