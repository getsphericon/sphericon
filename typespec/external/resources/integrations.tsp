using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A per-second Send rate limit the caller may set */
@minValue(1)
@maxValue(10000)
scalar MaxPerSecond extends int32;

/** A rolling 24-hour Send rate limit the caller may set */
@minValue(1)
@maxValue(100000000)
scalar MaxPerDay extends int32;

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

  /** The one SNS topic bounce and complaint notifications are accepted from; none means the hook refuses all */
  snsTopicArn?: string | null;

  /** Last 4 chars of the access key id, for recognition */
  accessKeyIdLast4?: string | null;
}

/** Provider config as returned, discriminated by `kind`. Secrets are omitted. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union IntegrationConfig {
  smtp: SmtpConfig,
  ses: SesConfig,
}

// --- Input config (carries secrets; write-only) ---

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

  /** The SNS topic (ARN) this Integration's bounces and complaints arrive through; notifications from any other topic are refused */
  snsTopicArn?: string | null;
}

/** Provider config submitted by the caller, discriminated by `kind`. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union IntegrationConfigInput {
  smtp: SmtpConfigInput,
  ses: SesConfigInput,
}

// --- Resource + request bodies ---

/** A sending-provider Integration. Credentials are write-only: the config is returned with secrets redacted. */
@resource("integrations")
@segment("integrations")
model IntegrationResource {
  /** Unique identifier */
  @key
  id: EntityId;

  /** Human-readable label */
  @extension("x-untrusted", true)
  name: string;

  /** Delivery channel */
  channel: IntegrationChannel;

  /** Concrete provider */
  provider: IntegrationProvider;

  /** Whether this integration may be used for sending */
  enabled: boolean;

  /** Whether this is the default provider for its channel */
  isDefault: boolean;

  /** Send rate limit set on the Integration: most messages per second; null means none */
  maxPerSecond: int32 | null;

  /** Send rate limit set on the Integration: most messages per rolling 24 hours; null means none */
  maxPerDay: int32 | null;

  /** The Send rate limit as enforced and 24-hour usage */
  sendLimit: SendLimitStatus;

  /** Provider config (secrets redacted) */
  config: IntegrationConfig;

  /** Creation timestamp */
  createdAt: Timestamp;

  /** Last update timestamp */
  updatedAt: Timestamp;
}

/** Create an Integration. The provider and channel are derived from `config.kind`. */
model CreateIntegrationInput {
  name: string;
  enabled?: boolean;
  isDefault?: boolean;

  /** Most messages per second (1 to 10 000); omit or null for no limit */
  maxPerSecond?: MaxPerSecond | null;

  /** Most messages per rolling 24 hours (1 to 100 000 000); omit or null for no limit */
  maxPerDay?: MaxPerDay | null;

  config: IntegrationConfigInput;
}

/** Update an Integration. Omit `config` to keep stored credentials; blank secret fields inside it keep the stored secret. */
model UpdateIntegrationInput {
  name?: string;
  enabled?: boolean;
  isDefault?: boolean;

  /** Most messages per second (1 to 10 000); null clears the limit, omit to keep */
  maxPerSecond?: MaxPerSecond | null;

  /** Most messages per rolling 24 hours (1 to 100 000 000); null clears the limit, omit to keep */
  maxPerDay?: MaxPerDay | null;

  config?: IntegrationConfigInput | null;
}

@tag("Integrations")
interface Integrations
  extends ExternalCrudUnique<
      IntegrationResource,
      CreateIntegrationInput,
      UpdateIntegrationInput
    > {}
