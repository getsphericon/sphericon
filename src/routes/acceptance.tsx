import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

import { isGated, requireUser } from '../currentUser.ts'
import { zReturnPath } from '../utils/returnPath.ts'
import { Route as rootRoute } from './__root.tsx'

export const Route = createFileRoute('/acceptance')({
  validateSearch: z.object({
    // The same-origin page the gate stopped the User on (an OAuth consent, an Invitation), to
    // resume once they accept.
    redirect: zReturnPath,
  }),
  // The accept-or-sign-out screen (ADR 0062): a signed-in User with a required acceptance pending
  // stays; anyone else goes on to the page they asked for, or to the app, whose own guard decides
  // where. The route imports no guarded route (the guards import this one), so the way on is the
  // root.
  beforeLoad: async ({ context, search }) => {
    const me = await requireUser(context.queryClient)
    if (!isGated(me)) {
      throw redirect(search.redirect ? { href: search.redirect } : { to: rootRoute.to })
    }
  },
})
