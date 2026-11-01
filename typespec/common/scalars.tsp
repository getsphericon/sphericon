namespace Sphericon;

@format("email")
@maxLength(320)
scalar EmailAddress extends string;

@format("date-time")
scalar Timestamp extends string;

scalar TimeZoneName extends string;

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
 * A primary key, written as a decimal string so JavaScript never rounds it. The format
 * makes the Go servers parse it into an int64: a malformed id is refused as a 400 before
 * a handler runs. The pattern says the same in the contract, so a client (or a fuzzer) can
 * tell a malformed id from a valid one without trying it. Eighteen digits is the most a key
reaches, so every id the schema admits fits an int64.
 */
@format("int64")
@pattern("^[0-9]{1,18}$")
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
model JsonObject is Record<unknown>;

/**
 * The email editor's JSON document, an opaque object the server checks against the schema
 * `email_schema` returns. One named model, as JsonObject.
 */
model EditorDocument is Record<unknown>;
