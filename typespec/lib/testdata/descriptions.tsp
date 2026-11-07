// A description says what happens to a value, never which code does it: one that names a
// module, a package, a use case, a Go identifier or a Go source path is refused, wherever it is
// written, and once, where it is written.
import "../main.tsp";

using Sphericon;

namespace Fixture;

// expect-next-line: sphericon/implementation-in-description
/** A Contact, kept by the contacts module */
model Contact {
  // expect-next-line: sphericon/implementation-in-description
  /** The address, as typed: trimmed and validated by the erasure module */
  email: string;

  // expect-next-line: sphericon/implementation-in-description
  /** Normalized by throttle.NormalizeEmail before the lookup */
  login: string;

  // expect-next-line: sphericon/implementation-in-description
  /** Checked as in internal/usecase/contacterasure */
  phone: string;

  // expect-next-line: sphericon/implementation-in-description
  /** Checked as lookup.go checks it */
  locale: string;

  /** The name, trimmed (e.g. "Ann"); a Login throttle delay does not apply to it */
  name: string;

  /** The snippet calls `window.Sphericon.track()` with `site.Contacts_lookup`, at v1.2 */
  snippet: string;
}

// expect-next-line: sphericon/implementation-in-description
@doc("Erase a Contact, through the contact erasure use case")
op erase(
  // expect-next-line: sphericon/implementation-in-description
  /** The email address to erase, checked by the package that erases */
  email: string,
): void;

// expect-next-line: sphericon/implementation-in-description
@summary("Look up, in the erasure Packages")
op lookup(): void;

// expect-next-line: sphericon/implementation-in-description
op list(): Contact[];

@@doc(list, "List the Contacts the use cases keep");

// expect-next-line: sphericon/implementation-in-description
/**
 * Count the Contacts
 * @returns the count the reports module keeps
 */
op count(): int32;

// expect-next-line: sphericon/implementation-in-description
// expect-next-line: sphericon/implementation-in-description
@returnsDoc("What the module returns")
@errorsDoc("What the package refuses")
op both(): void;

// expect-next-line: sphericon/implementation-in-description
/** Mark a Contact, as the marking module does */
op mark(): void;

op markAgain is mark;
op markOnceMore is mark;

model Copy is Contact;

model Spread {
  ...Contact;
}

interface Reads<T> {
  // expect-next-line: sphericon/implementation-in-description
  /** Read one, through the reading module */
  read(): T;
}

interface ContactReads extends Reads<Contact> {}
interface SpreadReads extends Reads<Spread> {}

/** Trimmed and lowercased, then refused (422) unless it is a single plain address */
scalar Address extends string;
