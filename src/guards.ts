import type { QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'

import { siteWorkspacesListOptions } from './generated/site/@tanstack/react-query.gen.ts'
import { Route as loginRoute } from './routes/login.tsx'
import { hasErrorStatus } from './utils/apiErrors.ts'

// The signed-in user's workspaces, read through the query cache the layouts share (a stale
// list is served at once and refreshed in the background). A 401 sends the visitor to the
// login, remembering `returnTo` when the guard guards a page worth coming back to.
export async function requireWorkspaces(queryClient: QueryClient, returnTo?: string) {
  try {
    return await queryClient.ensureQueryData({
      ...siteWorkspacesListOptions(),
      revalidateIfStale: true,
    })
  } catch (error) {
    if (!hasErrorStatus(error, [401])) throw error
    throw redirect(
      returnTo ? { to: loginRoute.to, search: { redirect: returnTo } } : { to: loginRoute.to },
    )
  }
}
