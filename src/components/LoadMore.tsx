import { Button, Stack } from '@mantine/core'
import type { UseSuspenseInfiniteQueryResult } from '@tanstack/react-query'

import { ApiErrorAlert } from './ApiErrorAlert.tsx'

interface LoadMoreProps {
  query: Pick<
    UseSuspenseInfiniteQueryResult,
    'error' | 'fetchNextPage' | 'hasNextPage' | 'isFetchingNextPage' | 'isFetchNextPageError'
  >
  label: string
  // The title of the alert a failed next page shows, with the API's detail under it.
  errorTitle: string
}

// LoadMore is the end of a list read page by page (an infinite query): while more exists, a button
// that loads the next page beneath the rows shown, spinning while it loads. A page that failed
// shows its alert above the button, which tries it again; the rows already shown stay.
export function LoadMore({ query, label, errorTitle }: LoadMoreProps) {
  if (!query.hasNextPage) {
    return null
  }
  return (
    <Stack gap="xs">
      {query.isFetchNextPageError ? <ApiErrorAlert error={query.error} title={errorTitle} /> : null}
      <Button
        variant="light"
        loading={query.isFetchingNextPage}
        onClick={() => {
          void query.fetchNextPage()
        }}
      >
        {label}
      </Button>
    </Stack>
  )
}
