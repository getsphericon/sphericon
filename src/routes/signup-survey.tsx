import { createFileRoute, redirect } from '@tanstack/react-router'

import { isGated, requireUser } from '../currentUser.ts'
import { Route as rootRoute } from './__root.tsx'
import { Route as acceptanceRoute } from './acceptance.tsx'

export const Route = createFileRoute('/signup-survey')({
  // A signed-in User who owes the survey stays; anyone else goes on to the app, whose own guard
  // decides where. The route imports no guarded route (the guards import this one), so the way
  // on is the root.
  beforeLoad: async ({ context }) => {
    const me = await requireUser(context.queryClient)
    // A required acceptance pending comes first (ADR 0062): the survey waits for it.
    if (isGated(me)) {
      throw redirect({ to: acceptanceRoute.to })
    }
    if (!me.signupSurveyDue) {
      throw redirect({ to: rootRoute.to })
    }
  },
})
