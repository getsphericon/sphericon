import { z } from 'zod'

// A search param that carries a link the server built: an absolute http(s) URL, anything else
// (a `javascript:` or `data:` URL, plain text) is dropped so a page never renders it as an href.
export const httpUrlParam = z
  .url({ protocol: /^https?$/ })
  .optional()
  .catch(undefined)

// The page of a paged list (PagedTable), so a page is linkable and Back returns to it. The first
// page is the absent param; anything unreadable falls back to it.
export const pageParam = z.coerce.number().int().min(2).optional().catch(undefined)

// The search of a move to another page of a paged list: the first page is the absent param.
export const withPage =
  (next: number) =>
  <TSearch extends object>(prev: TSearch) => ({ ...prev, page: next > 1 ? next : undefined })
