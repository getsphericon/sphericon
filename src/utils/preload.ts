import { noop } from '@tanstack/react-query'
import type { QueryClient, QueryExecuteOptions, QueryKey } from '@tanstack/react-query'

// preload is a route loader's read of what its page shows, so a hovered link opens the page with
// its data. A record already cached (the hover loaded it) is used as it is, not fetched again on
// the click; a failure is left to the page, whose QueryBoundary shows it inline.
export async function preload<TQueryFnData, TError, TData, TQueryKey extends QueryKey>(
  queryClient: QueryClient,
  options: QueryExecuteOptions<TQueryFnData, TError, TData, TQueryFnData, TQueryKey>,
): Promise<void> {
  await queryClient.query({ ...options, staleTime: 'static' }).catch(noop)
}
