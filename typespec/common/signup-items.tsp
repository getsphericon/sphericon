namespace Sphericon;

/** What a Sign-up item asks of a person (ADR 0062) */
enum SignupItemKind {
  /** A checkbox without which no User is created */
  requiredAcceptance: "required_acceptance",

  /** An unticked checkbox sign-up never depends on */
  optionalConsent: "optional_consent",

  /** A link to read, nothing to tick */
  notice: "notice",
}

/** A Sign-up item's key: its address, and the last segment of its Legal document's public page */
@minLength(1)
@maxLength(64)
@pattern("^[a-z0-9]+(-[a-z0-9]+)*$")
scalar SignupItemKey extends string;

/** A Sign-up item's title: what the sign-up form shows */
@minLength(1)
@maxLength(200)
scalar SignupItemTitle extends string;

/** The label a Staff member gives a published Legal document version, unique per item */
@minLength(1)
@maxLength(64)
scalar LegalDocumentLabel extends string;

/** A Legal document's text, in Markdown */
@maxLength(100000)
scalar LegalDocumentMarkdown extends string;
