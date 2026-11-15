import { skipToken } from '@tanstack/react-query'
import type { QueryFunction, SkipToken } from '@tanstack/react-query'

interface KeyHead {
  tags?: readonly string[]
}

// suspenseInfinite takes generated `…InfiniteOptions` for useSuspenseInfiniteQuery, given the
// plain query key of the same operation. The generated options type their function as possibly
// skipToken, which a suspense query refuses (a generated one is always a function; anything else
// throws), and their key carries none of the operation's tags, by which a write refreshes what it
// changed (invalidateOwn): the key takes the plain key's tags.
export function suspenseInfinite<TQueryFnData, THead extends KeyHead, TPageParam>(
  {
    queryKey: [head],
    queryFn,
  }: {
    queryKey: [THead]
    queryFn?: QueryFunction<TQueryFnData, [THead], TPageParam> | SkipToken
  },
  [{ tags }]: readonly [KeyHead],
) {
  if (queryFn === undefined || queryFn === skipToken) {
    throw new Error('A suspense query is read with a query function')
  }
  const queryKey: [THead] = [{ ...head, ...(tags && { tags }) }]
  return { queryKey, queryFn }
}
