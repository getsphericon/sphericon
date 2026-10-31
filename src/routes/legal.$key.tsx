import { createFileRoute } from '@tanstack/react-router'

import { siteLegalDocumentsGetOptions } from '../generated/site/@tanstack/react-query.gen.ts'

// The public page of a Sign-up item's Legal document (ADR 0062): no sign-in.
export const Route = createFileRoute('/legal/$key')({
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline. An
  // unknown key is an answer, not a fault, so it is not retried, as on the page.
  loader: async ({ context, params }) => {
    await context.queryClient.prefetchQuery({
      ...siteLegalDocumentsGetOptions({ path: { key: params.key } }),
      retry: false,
    })
  },
})
