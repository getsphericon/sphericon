using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace Sphericon;

/** A User as the feed reports it, with their current consent to the operator's mailings
 * (ADR 0062). A deleted User is a tombstone: only its id and when it was deleted, nothing
 * else of theirs is kept. */
model ServiceUserResource {
  /** The User id */
  id: EntityId;

  /** The User's current name; absent on a tombstone */
  name?: string;

  /** The User's current email; absent on a tombstone */
  email?: EmailAddress;

  /** When the User confirmed their email; absent while it is unverified and on a tombstone */
  emailVerifiedAt?: Timestamp | null;

  /** When the User was created; absent on a tombstone */
  createdAt?: Timestamp;

  /** When the User was deleted; absent while the User exists. A tombstone carries nothing
   * else: the control plane erases the User from its mailing list. */
  deletedAt?: Timestamp | null;

  /** Whether the User's latest decision on the Sign-up item that stands for consent to the
   * operator's mailings is an acceptance. False when they declined or withdrew it, never
   * answered it, on a tombstone, and when no item stands for it. */
  mailingsConsent: boolean;

  /** When the User made that latest decision; absent when they never answered the item */
  mailingsConsentChangedAt?: Timestamp | null;
}

/** One page of the Users feed */
model ServiceUserFeedPage {
  /** Users created or changed after the requested cursor, in change order */
  items: ServiceUserResource[];

  /** Pass it back as the cursor to read what follows this page. It is the requested
   * cursor itself when the page is empty. Saving it is what advances the caller: the
   * core keeps no position of its own. */
  nextCursor: string;

  /** Whether more entries follow this page right now */
  hasMore: boolean;
}

@tag("Service Users")
@route("/users/changes")
namespace ServiceUserFeed {
  /** The Users created or changed since an opaque cursor, each with their current consent
   * to the operator's mailings, in a stable order by change position. A change is a new
   * User, a new name, email or email verification, a new decision on the mailings item, the
   * mark of that item moving, or the User's deletion (a tombstone). Delivery is
   * at-least-once: replaying a cursor returns the same entries, and an empty cursor starts
   * from the beginning. */
  @get
  @requiresServiceScopes(#[ServiceScope.usersRead])
  op list(
    /** The `nextCursor` of a page the caller has saved; empty starts from the beginning */
    @query cursor?: string,

    /** Most entries on the page */
    @query
    @minValue(1)
    @maxValue(200)
    limit?: int32 = 100,
  ):
    | ServiceUserFeedPage
    | BadRequestProblem
    | UnauthorizedProblem
    | ForbiddenProblem
    | TooManyRequestsProblem;
}
