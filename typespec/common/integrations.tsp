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

/** Provider config submitted by the caller, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union IntegrationConfigInput {
  smtp: SmtpConfigInput,
  ses: SesConfigInput,
  inbox: InboxConfigInput,
}

/** Create an Integration. The provider and channel are derived from `config.kind`. A Test inbox (`inbox`) takes no credentials, never becomes the default by itself, and a second one is refused with 409. */
model CreateIntegrationInput {
  name: string;
  enabled?: boolean;
  isDefault?: boolean;

  /** Provider limit: most messages per second (1 to 10 000); omit or null for no limit, except on an SES Integration with an `endpoint`, which requires it */
  maxPerSecond?: MaxPerSecond | null;

  /** Provider limit: most messages per rolling 24 hours (1 to 100 000 000); omit or null for no limit, except on an SES Integration with an `endpoint`, which requires it */
  maxPerDay?: MaxPerDay | null;

  config: IntegrationConfigInput;

  /** Name of a Preset from `presets_list` to read provider callbacks (bounces, complaints) with */
  preset?: string | null;

  /** The Preset's params, validated against its `paramsSchema`: read the schema first */
  params?: Record<unknown>;
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

  /** Name of a Preset from `presets_list`; null removes it. Changing it requires `params` in full: no secret carries over */
  preset?: string | null;

  /** The Preset's params. A secret that is blank, omitted or `********` stays as saved; a new value rotates it (the old one stays valid until retired) */
  params?: Record<unknown>;

  /** Stop accepting the retiring value of every rotating secret */
  retireSecrets?: boolean;
}
