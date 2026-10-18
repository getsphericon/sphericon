using TypeSpec.Http;
using TypeSpec.Rest;
using TypeSpec.Rest.Resource;
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

  /** Most messages per second (1 to 10 000); null clears the limit, omit to keep */
  maxPerSecond?: MaxPerSecond | null;

  /** Most messages per rolling 24 hours (1 to 100 000 000); null clears the limit, omit to keep */
  maxPerDay?: MaxPerDay | null;

  config?: IntegrationConfigInput | null;

  /** Name of a Preset from `presets_list`; null removes it. Changing it requires `params` in full: no secret carries over */
  preset?: string | null;

  /** The Preset's params. A secret that is blank, omitted or `********` stays as saved; a new value rotates it (the old one stays valid until retired) */
  params?: Record<unknown>;

  /** Stop accepting the retiring value of every rotating secret */
  retireSecrets?: boolean;
}

@tag("Integrations")
interface Integrations
  extends ExternalCrudUnique<
      IntegrationResource,
      CreateIntegrationInput,
      UpdateIntegrationInput
    > {
  /** Replace the Integration's hook key: the old hook URL stops working at once and the new one must be pasted into the provider. Needs a person: hidden from MCP. */
  @action("regenerate-key")
  @autoRoute
  regenerateKey(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;
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
  paramsSchema: Record<unknown>;
}

@route("/presets")
@tag("Integrations")
namespace Presets {
  /** List the Presets the Instance offers, with the params schema of each */
  @get
  op list():
    | Page<PresetResource>
    | UnauthorizedProblem
    | TooManyRequestsProblem;
}
