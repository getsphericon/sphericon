namespace Sphericon;

@format("email")
@maxLength(320)
scalar EmailAddress extends string;

@format("date-time")
scalar Timestamp extends string;

scalar TimeZoneName extends string;

/**
 * A primary key, written as a decimal string so JavaScript never rounds it. The format
 * makes the Go servers parse it into an int64: a malformed id is refused as a 400 before
 * a handler runs.
 */
@format("int64")
scalar EntityId extends string;
