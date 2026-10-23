import { createFileRoute, redirect } from '@tanstack/react-router'

import { requireUser } from '../currentUser.ts'
import { Route as rootRoute } from './__root.tsx'

export const Route = createFileRoute('/signup-survey')({
  // A signed-in User who owes the survey stays; anyone else goes on to the app, whose own guard
  // decides where. The route imports no guarded route (the guards import this one), so the way
  // on is the root.
  beforeLoad: async ({ context }) => {
    const me = await requireUser(context.queryClient)
    if (!me.signupSurveyDue) {
      throw redirect({ to: rootRoute.to })
    }
  },
})
