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
 * a handler runs. The pattern says the same in the contract, so a client (or a fuzzer) can
 * tell a malformed id from a valid one without trying it. Eighteen digits is the most a key
reaches, so every id the schema admits fits an int64.
 */
@format("int64")
@pattern("^[0-9]{1,18}$")
scalar EntityId extends string;
