import { createFileRoute, redirect } from '@tanstack/react-router'

import { ConsoleLayout } from '../../../console/layout.tsx'
import { Route as consoleLoginRoute } from '../login.tsx'

export const Route = createFileRoute('/console/_authed')({
  // Guarded by the Operator session: anything but a signed-in Operator (no cookie, an expired
  // one, or no license, where the whole surface answers 404) goes to the login.
  beforeLoad: async () => {
    const { operatorMeGet } = await import('../../../generated/operator/sdk.gen.ts')
    const { data } = await operatorMeGet()
    if (!data) throw redirect({ to: consoleLoginRoute.to })
    return { operator: data }
  },
  component: ConsoleLayout,
})
