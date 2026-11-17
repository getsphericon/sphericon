// The Integration models /site and /api show and take the same way: its Channel, provider and
// Provider limit, its config as returned and as written, its write bodies and the Presets.

namespace Sphericon;

/** A per-second Provider limit the caller may set */
@minValue(1)
@maxValue(10000)
scalar MaxPerSecond extends int32;

/** SMTP credentials */
model SmtpConfigInput {
  kind: "smtp";
  host: string;
  port: int32;
  username?: string | null;
  password?: string | null;
  from: EmailAddress;
  fromName?: string | null;
}

/** SES credentials */
model SesConfigInput {
  kind: "ses";
  region: string;
  accessKeyId: string;
  secretAccessKey: string;
  from: EmailAddress;
  fromName?: string | null;

  /** Custom SES-compatible API endpoint (e.g. Yandex Cloud Postbox); empty for AWS */
  endpoint?: string | null;
}

/** The Test inbox takes no credentials: a Workspace has at most one */
model InboxConfigInput {
  kind: "inbox";
}

/** A Telegram bot of the Workspace's own, on the `telegram` channel. Saving it checks the token with Telegram and points the bot's webhook at the integration's hook, replacing any webhook the bot had: use a bot dedicated to sphericon */
model TelegramConfigInput {
  kind: "telegram";

  /** The bot token @BotFather gives (`123456789:AA…`). Blank on an update keeps the stored one; a token of another bot is refused */
  token: string;
}

/** A Workspace's own Twilio account on the `sms` channel. Saving it reads the sender with the credentials (a wrong or revoked key is a 422) and points the sender's inbound messages, and a Messaging Service's delivery reports, at the integration's hook. The Auth Token that signs Twilio's callbacks is the `twilio` Preset's `auth_token` param, kept and rotated apart from these credentials */
model TwilioConfigInput {
  kind: "twilio";

  /** The Account SID (`AC…`) */
  accountSid: string;

  /** An API key's SID (`SK…`) to send with; omit or null to send with the Account SID and its Auth Token */
  apiKeySid?: string | null;

  /** The API key's secret, or the Auth Token when no API key is given. Write-only: blank on an update keeps the stored one while the Account SID and the API key stay the same */
  secret: string;

  /** What messages go out from: a phone number of the account in E.164 (`+15017250604`), an alphanumeric sender ID of up to 11 characters, or a Messaging Service SID (`MG…`) */
  from: string;
}

/** What an SMS integration's messages go out from */
enum SmsFromKind {
  /** A phone number of the account */
  number,

  /** An alphanumeric sender ID: one-way, replies do not reach it */
  alphanumeric,

  /** A Messaging Service of the account, which picks the number */
  messagingService,
}

/** Provider config submitted by the caller, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union IntegrationConfigInput {
  smtp: SmtpConfigInput,
  ses: SesConfigInput,
  inbox: InboxConfigInput,
  telegram: TelegramConfigInput,
  twilio: TwilioConfigInput,
}

/** Create an Integration. The provider and channel are derived from `config.kind`. A Test inbox (`inbox`) takes no credentials, never becomes the default by itself, and a second one is refused with 409. A Telegram bot (`telegram`) is checked with Telegram and its webhook set before the save; a refusal is a 422. A Twilio account (`twilio`) is checked with Twilio and its sender's callbacks set before the save; a refusal is a 422 */
model CreateIntegrationInput {
  name: Name;
  enabled?: boolean;
  isDefault?: boolean;

  /** Provider limit: most messages per second (1 to 10 000); omit or null for no limit, except on an SES Integration with an `endpoint`, which requires it */
  maxPerSecond?: MaxPerSecond | null;

  /** Provider limit: most messages per rolling 24 hours (1 to 100 000 000); omit or null for no limit, except on an SES Integration with an `endpoint`, which requires it */
  maxPerDay?: MaxPerDay | null;

  config: IntegrationConfigInput;

  /** Name of a Preset from `presets_list` to read provider callbacks (bounces, complaints) with. A Telegram integration takes none: it always reads its bot's updates with the built-in `telegram` Preset. A Twilio integration always reads its callbacks with the built-in `twilio` Preset, which no other integration takes */
  preset?: string | null;

  /** The Preset's params, validated against its `paramsSchema`: read the schema first. A Telegram integration takes none; a Twilio integration takes the `twilio` Preset's `auth_token`, required */
  params?: JsonObject;
}

/** Update an Integration. Omit `config` to keep stored credentials; blank secret fields inside it keep the stored secret. */
model UpdateIntegrationInput {
  /** The integration's `revision` the edit was made on (ADR 0119): the one it was read with */
  revision: Revision;

  name?: Name;
  enabled?: boolean;
  isDefault?: boolean;

  /** Provider limit: most messages per second (1 to 10 000); null clears the limit, omit to keep. An SES Integration with an `endpoint` cannot be left without it */
  maxPerSecond?: MaxPerSecond | null;

  /** Provider limit: most messages per rolling 24 hours (1 to 100 000 000); null clears the limit, omit to keep. An SES Integration with an `endpoint` cannot be left without it */
  maxPerDay?: MaxPerDay | null;

  config?: IntegrationConfigInput;

  /** Name of a Preset from `presets_list`; null removes it. Changing it requires `params` in full: no secret carries over. A Telegram or Twilio integration's Preset is fixed */
  preset?: string | null;

  /** The Preset's params. A secret that is blank, omitted or `********` stays as saved; a new value rotates it (the old one stays valid until retired): a Twilio integration's `auth_token` rotates this way, leaving its send credentials as they are. A Telegram integration takes none */
  params?: JsonObject;

  /** Stop accepting the retiring value of every rotating secret */
  retireSecrets?: boolean;
}

/** Delivery channel an integration belongs to */
enum IntegrationChannel {
  email,
  sms,

  /** A Workspace's own Telegram bot */
  telegram,
}

/** Concrete sending provider */
enum IntegrationProvider {
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
enum ProviderLimitSource {
  /** Set by the user on the Integration */
  manual,

  /** Reported by the provider */
  provider,
}

/** A reason to look at an Integration's Provider limit */
enum ProviderLimitWarning {
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
model ProviderLimitValue {
  /** The ceiling in messages; null means this window is not limited */
  limit: int32 | null;

  /** Where the ceiling comes from; null when there is none */
  source: ProviderLimitSource | null;
}

/** The Provider limit as enforced, with the last 24 hours of usage */
model ProviderLimitStatus {
  /** Most messages per second */
  perSecond: ProviderLimitValue;

  /** Most messages per rolling 24 hours */
  perDay: ProviderLimitValue;

  /** Messages the provider accepted from this Integration in the last 24 hours */
  sentLast24h: int32;

  /** Reasons to review the limit; empty when there is nothing to flag */
  warnings: ProviderLimitWarning[];
}

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

/** A Preset: how an Instance verifies and reads one provider's bounce and complaint callbacks */
model PresetResource {
  /** The name an Integration's `preset` refers to */
  name: string;

  /** What the Preset is for */
  description: string;

  /** JSON Schema (draft 2020-12) of the `params` an Integration of this Preset takes. A string marked `writeOnly` is a secret: it is never returned by a read. */
  paramsSchema: JsonObject;
}
