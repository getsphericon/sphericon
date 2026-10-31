import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/console/_authed/signup-items/$itemId')({
  // The item and its versions are fetched while the link is hovered; a failure is left to the
  // page, which shows it inline. The Staff client is imported here, as in the console's guard, so
  // it stays out of the app's entry chunk.
  loader: async ({ context, params }) => {
    const { staffSignupItemsGetOptions, staffSignupItemsListVersionsOptions } =
      await import('../../../generated/staff/@tanstack/react-query.gen.ts')
    const path = { id: params.itemId }
    await Promise.all([
      context.queryClient.prefetchQuery(staffSignupItemsGetOptions({ path })),
      context.queryClient.prefetchQuery(staffSignupItemsListVersionsOptions({ path })),
    ])
  },
})
