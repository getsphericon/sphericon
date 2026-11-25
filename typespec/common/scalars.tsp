import "@typespec/openapi";
import "../lib/main.tsp";

using TypeSpec.OpenAPI;

namespace Sphericon;

@format("email")
@maxLength(320)
scalar EmailAddress extends string;

/** What a person calls a named row (a Tag, a Segment, a Rule, an Automation, a Template, a
 * Broadcast, an Integration, a token, a Passkey, a Workspace): at most 100 characters. Surrounding
 * spaces are trimmed before the name is stored, and a name that is empty then is refused (422). */
@maxLength(100)
scalar Name extends string;

@format("date-time")
scalar Timestamp extends string;

/**
 * The revision of a resource a person edits (ADR 0119), starting at 1: every write of what it
 * shows moves it, a background job's record of its own (a delivery failure, a provider's reading)
 * does not.
 * The resource carries the one of the state it shows, and an update names the one it was made on:
 * a stale one is a 409 with code `revision_changed`, and nothing is written.
 */
@minValue(1)
scalar Revision extends integer;

/** The body of an editor's operation on stored content (Publish, Revert): the revision of what the
 * editor shows (ADR 0119) */
model RevisionInput {
  /** The resource's `revision` the operation was made on: the one it was read with */
  revision: Revision;
}

scalar TimeZoneName extends string;

/** A local time of day in whole minutes, `HH:MM` from 00:00 to 23:59, read in a time zone the
 * place that holds it names */
@pattern("^([01][0-9]|2[0-3]):[0-5][0-9]$")
scalar LocalTime extends string;

/** A BCP 47 tag, with its region, that a User picks for how the app shows dates, times and
 * numbers (`en-GB`, `de-DE`). A malformed tag is refused by the handler (422). */
@maxLength(35)
scalar RegionalFormat extends string;

/** An ISO 3166-1 alpha-2 country code, upper case (`US`, `DE`) */
@pattern("^[A-Z]{2}$")
scalar CountryCode extends string;

/** A BCP 47 language tag: a language, optionally with a script and a region (`de`, `pt-BR`) */
@pattern("^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$")
@maxLength(35)
scalar LocaleTag extends string;

/** The https URL of an image */
@pattern("^https://\\S+$")
@maxLength(2048)
scalar ImageUrl extends string;

/**
 * A primary key as a client sees it (ADR 0142): the key sealed under the Instance's id key and
 * written as 22 base62 characters, which say nothing about how many rows a table holds or when
 * one was made. The servers open it back into the key: a malformed id is refused as a 400 before
 * a handler runs. The pattern says the same in the contract, so a client (or a fuzzer) can tell a
 * malformed id from a valid one without trying it.
 */
@pattern("^[0-9A-Za-z]{22}$")
@extension(
  "x-ogen-type",
  "github.com/getsphericon/sphericon/internal/kernel/publicid.ID"
)
scalar EntityId extends string;

/**
 * A Workspace id (ADR 0043): 8 random base62 characters, assigned once at creation and the
 * Workspace's one public address.
 */
@pattern("^[0-9A-Za-z]{8}$")
scalar WorkspaceId extends string;

/**
 * A JSON object whose members the contract leaves open: custom field values, event properties,
 * integration params, an audit diff. One named model, so every surface's Go code has one type for
 * it rather than one per field.
 */
@openMap
model JsonObject is Record<unknown>;

/**
 * The email editor's JSON document, an opaque object the server checks against the schema
 * `email_schema` returns. One named model, as JsonObject.
 */
@openMap
model EditorDocument is Record<unknown>;
