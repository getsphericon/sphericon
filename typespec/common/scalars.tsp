namespace Sphericon;

@format("email")
@maxLength(320)
scalar EmailAddress extends string;

@format("date-time")
scalar Timestamp extends string;

scalar TimeZoneName extends string;

@pattern("^[0-9]+$")
scalar EntityId extends string;
