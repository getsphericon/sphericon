// The Contact write bodies: /site and /api take the same attributes.

namespace Sphericon;

/** Request body for creating a contact */
model CreateContactInput {
  /** The customer's own user id — an alias key */
  subjectId?: string | null;

  /** Email address — optional; any alias key may be absent */
  email?: EmailAddress | null;

  /** Phone number — an alias key */
  phone?: string | null;

  /** First name */
  firstName?: string | null;

  /** Last name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** ISO 3166-1 alpha-2 country code */
  country?: CountryCode | null;

  /** BCP 47 language tag of the language the contact reads in */
  locale?: LocaleTag | null;

  /** Date of birth */
  birthday?: plainDate | null;

  /** City */
  @maxLength(255)
  city?: string | null;

  /** https URL of the contact's photo */
  avatar?: ImageUrl | null;

  /** Typed custom field values keyed by the field's machine key. A new key's first value sets the
   * field's type: a boolean, a number, a date (an ISO string: `2026-01-15T10:00:00Z`,
   * `2026-01-15T10:00:00` or `2026-01-15`, the last two read in the Workspace time zone) or text.
   * Send a date without time as a `YYYY-MM-DD` string, not a JavaScript `Date`: that serializes
   * as UTC and lands on the previous day east of UTC */
  customFields?: JsonObject | null;
}

/** Request body for updating a contact */
model UpdateContactInput {
  /** The customer's own user id — an alias key */
  subjectId?: string | null;

  /** Email address — an alias key */
  email?: EmailAddress | null;

  /** Phone number — an alias key */
  phone?: string | null;

  /** First name */
  firstName?: string | null;

  /** Last name */
  lastName?: string | null;

  /** IANA time zone identifier */
  timeZone?: TimeZoneName | null;

  /** ISO 3166-1 alpha-2 country code */
  country?: CountryCode | null;

  /** BCP 47 language tag of the language the contact reads in */
  locale?: LocaleTag | null;

  /** Date of birth */
  birthday?: plainDate | null;

  /** City */
  @maxLength(255)
  city?: string | null;

  /** https URL of the contact's photo */
  avatar?: ImageUrl | null;

  /** Typed custom field values keyed by the field's machine key. A new key's first value sets the
   * field's type: a boolean, a number, a date (an ISO string: `2026-01-15T10:00:00Z`,
   * `2026-01-15T10:00:00` or `2026-01-15`, the last two read in the Workspace time zone) or text.
   * Send a date without time as a `YYYY-MM-DD` string, not a JavaScript `Date`: that serializes
   * as UTC and lands on the previous day east of UTC */
  customFields?: JsonObject | null;
}
