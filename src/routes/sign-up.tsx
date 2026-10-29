import { createFileRoute, redirect } from '@tanstack/react-router'

import { isSignedIn } from '../signedIn.ts'
import { Route as rootRoute } from './__root.tsx'

export const Route = createFileRoute('/sign-up')({
  // A signed-in visitor has an account already: on to the app.
  beforeLoad: async ({ context }) => {
    if (await isSignedIn(context.queryClient)) {
      throw redirect({ to: rootRoute.to })
    }
  },
})
