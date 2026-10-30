import type { QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'

import { siteUserGetMeOptions } from './generated/site/@tanstack/react-query.gen.ts'
import type { SiteUserResource } from './generated/site/types.gen.ts'
import { Route as loginRoute } from './routes/login.tsx'
import { hasErrorStatus } from './utils/apiErrors.ts'

// Whether the acceptance gate stops the User (ADR 0062): a required acceptance has a current
// version they have not accepted.
export function isGated(me: SiteUserResource) {
  return me.acceptancesPending.length > 0
}

// The signed-in user, read through the query cache. A 401 sends the visitor to the login,
// remembering `returnTo` when the guard guards a page worth coming back to. This module imports
// no route that has a guard, so the survey's own route can use it without a cycle.
export async function requireUser(queryClient: QueryClient, returnTo?: string) {
  try {
    const options = siteUserGetMeOptions()
    const cached = queryClient.getQueryData(options.queryKey)
    // A cached "due" or pending acceptance is never served: answering the survey or accepting
    // leaves the cache behind, and a stale one would send the User back to its screen.
    if (cached === undefined || cached.signupSurveyDue || isGated(cached)) {
      return await queryClient.query(options)
    }
    // Serve the cached user at once; a stale one is refreshed behind it.
    void queryClient.query(options).catch(() => {
      // A failed background refresh keeps the cached user; the next read retries.
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
