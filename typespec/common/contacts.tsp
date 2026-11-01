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

  /** Typed custom field values keyed by the field's machine key */
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

  /** Typed custom field values keyed by the field's machine key */
  customFields?: JsonObject | null;
}
