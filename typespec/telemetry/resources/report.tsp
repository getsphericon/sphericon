using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A count rounded to its order of magnitude, never an exact number. */
enum OrderOfMagnitude {
  zero: "0",
  oneToNine: "1-9",
  tenToNinetyNine: "10-99",
  hundredToNineHundredNinetyNine: "100-999",
  thousands: "1000-9999",
  tenThousands: "10000-99999",
  hundredThousands: "100000-999999",
  millionOrMore: "1000000+",
}

enum TelemetryEdition {
  core,
  ee,
}

enum TelemetryDeployShape {
  binary,
  container,
  kubernetes,
  `unknown`,
}

enum TelemetryLocale {
  ru,
  en,
  es,
}

enum TelemetryIntegrationKind {
  smtp,
  ses,
}

/**
 * What an Instance reports about itself. It carries no name, slug, domain, address, host, error
 * text, licence key or exact number, and nothing that identifies a Workspace or a person. A new
 * field is a change to this schema and to its documentation in the same commit.
 */
@extension("additionalProperties", false)
model TelemetryReport {
  /** The version of this schema the report was built against. */
  @minValue(1)
  schemaVersion: int32;

  /** A random value stored in the Instance's database, derived from nothing. */
  @minLength(1)
  @maxLength(64)
  instanceId: string;

  /** The release version of the running binary, for example `1.4.2`. */
  @minLength(1)
  @maxLength(64)
  appVersion: string;

  edition: TelemetryEdition;

  /** The operating system the binary runs on (Go's GOOS), for example `linux`. */
  @minLength(1)
  @maxLength(32)
  os: string;

  /** The CPU architecture the binary runs on (Go's GOARCH), for example `arm64`. */
  @minLength(1)
  @maxLength(32)
  arch: string;

  deployShape: TelemetryDeployShape;

  /** The major version of the Postgres server, for example `17`. */
  @minValue(1)
  postgresMajor: int32;

  /** The Instance's configured locale. */
  locale: TelemetryLocale;

  /** Whole days since the Instance was first started. */
  @minValue(0)
  instanceAgeDays: int32;

  workspaces: OrderOfMagnitude;

  /** Identified Contacts, excluding erased ones. */
  contacts: OrderOfMagnitude;

  /** Emails sent in the last 30 days. */
  emailsSent30d: OrderOfMagnitude;

  /** A Broadcast was sent in the last 30 days. */
  usedBroadcasts30d: boolean;

  /** At least one Automation is active. */
  automationsActive: boolean;

  /** The tracker recorded a visit in the last 30 days. */
  usedTracker30d: boolean;

  /** An external API token was used in the last 30 days. */
  usedExternalApi30d: boolean;

  /** At least one webhook endpoint is configured. */
  webhooksConfigured: boolean;

  /** The kinds of sending Integration in use, each at most once. */
  @maxItems(8)
  integrationKinds: TelemetryIntegrationKind[];
}

@route("/v1/reports")
@tag("Telemetry")
namespace TelemetryReports {
  /** Receives one Telemetry report. The receiver stores no source address. */
  @post
  op submit(@body body: TelemetryReport): NoContentResponse;
}
