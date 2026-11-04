// The Integration write bodies: /site and /api take the same input.

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
  name: string;
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
  name?: string;
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
