import { createFileRoute, redirect } from '@tanstack/react-router'

import { ConsoleLayout } from '../../../console/layout.tsx'
import { Route as consoleLoginRoute } from '../login.tsx'

export const Route = createFileRoute('/console/_authed')({
  // Guarded by the Staff member session: anything but a signed-in Staff member (no cookie, an expired
  // one, or no license, where the whole surface answers 404) goes to the login.
  beforeLoad: async () => {
    const { staffMeGet } = await import('../../../generated/staff/sdk.gen.ts')
    const { data } = await staffMeGet()
    if (!data) {
      throw redirect({ to: consoleLoginRoute.to })
    }
    return { staff: data }
  },
  component: ConsoleLayout,
})
