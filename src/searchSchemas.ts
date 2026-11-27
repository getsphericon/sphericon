import { z } from 'zod'

import { zSiteSigninError } from './generated/site/zod.gen.ts'

// A search param that carries a link the server built: an absolute http(s) URL, anything else
// (a `javascript:` or `data:` URL, plain text) is dropped so a page never renders it as an href.
export const httpUrlParam = z
  .url({ protocol: /^https?$/ })
  .optional()
  .catch(undefined)

// The token of a mailed link the page sends on; an absent or unreadable one is empty, which the
// server refuses as it does a spent link.
export const tokenParam = z.string().catch('')

// Why a sign-in or a link with a provider did not succeed, set by the server's callback; a value
// the contract does not name is dropped.
export const signinErrorParam = zSiteSigninError.optional().catch(undefined)

// The page of a paged list (PagedTable), so a page is linkable and Back returns to it. The first
// page is the absent param; anything unreadable falls back to it.
export const pageParam = z.coerce.number().int().min(2).optional().catch(undefined)

// The search of a paged list page.
export const pagedSearch = z.object({ page: pageParam })

// The search of a list with an archived view (ArchiveViewControl): the archived rows are shown
// while `archived` is set.
export const archivableSearch = pagedSearch.extend({
  archived: z.boolean().optional().catch(undefined),
})

// The search of a move to another page of a paged list: the first page is the absent param.
export const withPage =
  (next: number) =>
  <TSearch extends object>(prev: TSearch) => ({ ...prev, page: next > 1 ? next : undefined })
