import { queryOptions } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'

import { hasErrorStatus } from './apiErrors.ts'

// oneShotTokenQuery reads what the token of a link that works once yields: `send` answers, and a
// refused link (400: spent, garbage or expired) answers `spent` rather than failing. A POST has no
// generated query key, so the key is its mutation key, which names the operation and the token; a
// Site Auth mutation invalidates it by that key's tag, which a static query never refetches. The
// answer is kept for good (the token is spent by it), so a remount or a refocus never sends the
// token again, and `send` takes no abort signal, so a request that was sent is not cut off and
// sent a second time.
export function oneShotTokenQuery<TQueryKey extends QueryKey, TAnswer, TSpent>(
  queryKey: TQueryKey,
  send: () => Promise<TAnswer>,
  spent: TSpent,
) {
  return queryOptions({
    queryKey,
    queryFn: async () => {
      try {
        return await send()
      } catch (error) {
        if (hasErrorStatus(error, [400])) {
          return spent
        }
        throw error
      }
    },
    staleTime: 'static',
    retry: false,
  })
}
