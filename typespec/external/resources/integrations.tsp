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

/** Provider config as returned, discriminated by `kind`. Secrets are omitted. */
@discriminated(#{ envelope: "none", discriminatorPropertyName: "kind" })
union IntegrationConfig {
  smtp: SmtpConfig,
  ses: SesConfig,
  inbox: InboxConfig,
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
model IntegrationsListFilter {
  /** true lists the archived integrations instead of the kept ones; the default hides them */
  @query archived?: boolean;
}

/**
 * An integration is archived, never deleted (ADR 0049): its sent mail keeps the channel it went
 * through. Not built on ExternalCrud, which carries a delete.
 */
@tag("Integrations")
interface Integrations {
  /** List integrations with pagination */
  @listsResource(IntegrationResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  list(
    ...ResourceCollectionParameters<IntegrationResource>,
    ...PageQuery,
    ...IntegrationsListFilter,
  ):
    | Page<IntegrationResource>
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | UnprocessableEntityProblem;

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
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Get an integration by ID, archived or not */
  @readsResource(IntegrationResource)
  @get
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  get(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  /** Update an integration */
  @updatesResource(IntegrationResource)
  @put
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  update(
    ...ResourceParameters<IntegrationResource>,
    @body body: UpdateIntegrationInput,
  ):
    | IntegrationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem
    | UnprocessableEntityProblem;

  /** Archive an integration: it leaves the list and gives up the default, and can be restored (it does not become the default again). Refused with 409 while a scheduled or sending broadcast, or an active email automation through the default, uses it (`usedBy` names them). */
  @action("archive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  archive(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | InUseProblem;

  /** Restore an archived integration to the list. It is not the default; promote it with an update */
  @action("unarchive")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  unarchive(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem
    | NotFoundProblem
    | ConflictProblem;

  /** Replace the Integration's hook key: the old hook URL stops working at once and the new one must be pasted into the provider. Needs a person: hidden from MCP. */
  @action("regenerate-key")
  @autoRoute
  @requiresScopes(#[ApiTokenScope.integrationsWrite])
  regenerateKey(...ResourceParameters<IntegrationResource>):
    | IntegrationResource
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
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
  @requiresScopes(#[ApiTokenScope.integrationsRead])
  op list():
    | Page<PresetResource>
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;
}
