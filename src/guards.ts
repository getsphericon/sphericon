import type { QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'

import { requireUser } from './currentUser.ts'
import { siteWorkspacesListOptions } from './generated/site/@tanstack/react-query.gen.ts'
import { Route as loginRoute } from './routes/login.tsx'
import { Route as signupSurveyRoute } from './routes/signup-survey.tsx'
import { hasErrorStatus } from './utils/apiErrors.ts'

// Their workspaces, read through the cache the layouts share (a stale list is served at once and
// refreshed in the background).
async function workspacesFor(queryClient: QueryClient, returnTo?: string) {
  try {
    const options = siteWorkspacesListOptions()
    const cached = queryClient.getQueryData(options.queryKey)
    if (cached === undefined) {
      return await queryClient.query(options)
    }
    // Serve the cached list at once; a stale one is refreshed behind it.
    void queryClient.query(options).catch(() => {
      // A failed background refresh keeps the cached list; the next read retries.
    })
    return cached
  } catch (error) {
    if (!hasErrorStatus(error, [401])) {
      throw error
    }
    throw redirect(
      returnTo ? { to: loginRoute.to, search: { redirect: returnTo } } : { to: loginRoute.to },
    )
  }
}

// The signed-in user's workspaces for a guarded page. A 401 sends the visitor to the login,
// remembering `returnTo` when the guard guards a page worth coming back to, and a User who owes
// the Sign-up survey is sent to it before any page of the app, whatever page was asked for.
export async function requireWorkspaces(queryClient: QueryClient, returnTo?: string) {
  const me = await requireUser(queryClient, returnTo)
  if (me.signupSurveyDue) {
    throw redirect({ to: signupSurveyRoute.to })
  }
  return await workspacesFor(queryClient, returnTo)
}
