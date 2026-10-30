import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteSigninError } from '../generated/site/zod.gen.ts'
import { isSignedIn } from '../signedIn.ts'
import { zReturnPath } from '../utils/returnPath.ts'
import { Route as rootRoute } from './__root.tsx'

export const Route = createFileRoute('/login')({
  validateSearch: z.object({
    // A same-origin path to return to after signing in.
    redirect: zReturnPath,
    // Why a sign-in with a provider did not start a session (set by the server's callback).
    error: zSiteSigninError.optional().catch(undefined),
    // The Second factor challenge of a provider sign-in (set by the server's callback).
    challenge: z.string().optional().catch(undefined),
  }),
  // A signed-in visitor goes on to the app (or where they were headed). A provider sign-in coming
  // back with an error or a Second factor challenge stays: it is still finishing a sign-in.
  beforeLoad: async ({ context, search }) => {
    if (search.error || search.challenge || !(await isSignedIn(context.queryClient))) {
      return
    }
    throw redirect(search.redirect ? { href: search.redirect } : { to: rootRoute.to })
  },
})
