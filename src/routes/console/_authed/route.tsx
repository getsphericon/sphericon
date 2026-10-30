import { createFileRoute, redirect } from '@tanstack/react-router'

import { Route as consoleLoginRoute } from '../login.tsx'

export const Route = createFileRoute('/console/_authed')({
  // Guarded by the Staff member session: anything but a signed-in Staff member (no cookie, an
  // expired one, or an Instance without the Staff sign-in, where the whole surface answers 404)
  // goes to the login. The console's Enterprise sections show only when the server serves them:
  // without the license their routes answer 404 (ADR 0063), read once per sign-in.
  beforeLoad: async ({ context }) => {
    const { staffMeGet } = await import('../../../generated/staff/sdk.gen.ts')
    const { data } = await staffMeGet()
    if (!data) {
      throw redirect({ to: consoleLoginRoute.to })
    }
    const enterprise = await context.queryClient.ensureQueryData({
      queryKey: ['console', 'enterpriseSections'],
      queryFn: async () => {
        const { staffWorkspacesList } = await import('../../../generated/staffee/sdk.gen.ts')
        const { response } = await staffWorkspacesList({ query: { pageSize: 1 } })
        return response?.status !== 404
      },
      staleTime: Infinity,
    })
    return { staff: data, enterprise }
  },
})
