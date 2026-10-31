import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/console/_authed/_enterprise/workspaces/$workspaceId')({
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline. The
  // Staff client is imported here, as in the console's guard, so it stays out of the app's entry
  // chunk.
  loader: async ({ context, params }) => {
    const { staffWorkspacesGetOptions } =
      await import('../../../../generated/staffee/@tanstack/react-query.gen.ts')
    await context.queryClient.prefetchQuery(
      staffWorkspacesGetOptions({ path: { workspaceId: params.workspaceId } }),
    )
  },
})
