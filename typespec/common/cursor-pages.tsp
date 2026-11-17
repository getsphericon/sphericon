using TypeSpec.Http;

namespace Sphericon;

// The pages of a list read by cursor. This file declares only templates and aliases, which are
// emitted where they are used, so a surface that imports it carries no schema it does not use.
//
// A list read by cursor declares its page as `model XList is CursorPage<X>` (or `FeedPage`), so
// the schema keeps its own name, and spreads the matching query alias. A query is an alias, not a
// model: a query parameter spread from a named model is emitted as a shared parameter named after
// that model, and these are emitted inline in each operation.

/** A page of a list read by cursor that ends: `nextCursor` is absent on the last page */
model CursorPage<T> {
  items: T[];

  /** Pass as `cursor` for the next page; absent on the last page */
  nextCursor?: string | null;
}

/** The query of a list read by `CursorPage` */
alias CursorQuery = {
  /** Cursor from a previous page */
  @query cursor?: string;

  /** Page size */
  @query
  @minValue(1)
  @maxValue(100)
  limit?: int32 = 25;
};

/** A page of a feed a caller follows for good: `nextCursor` is always set, the requested cursor
 * itself on an empty page. `ItemsDoc` describes the entries. */
model FeedPage<T, ItemsDoc extends valueof string> {
  @doc(ItemsDoc)
  items: T[];

  /** Pass it back as the cursor to read what follows this page. It is the requested
   * cursor itself when the page is empty. Saving it is what advances the caller: the
   * core keeps no position of its own. */
  nextCursor: string;

  /** Whether more entries follow this page right now */
  hasMore: boolean;
}

/** The query of a feed read by `FeedPage` */
alias FeedQuery = {
  /** The `nextCursor` of a page the caller has saved; empty starts from the beginning */
  @query cursor?: string;

  /** Most entries on the page */
  @query
  @minValue(1)
  @maxValue(200)
  limit?: int32 = 100;
};
