import { createFileRoute, redirect } from '@tanstack/react-router'

import { requireWorkspaces } from '../guards.ts'
import { Route as loginRoute } from './login.tsx'
import { Route as overviewRoute } from './workspaces/$slug/index.tsx'

export const Route = createFileRoute('/')({
  beforeLoad: async ({ context }) => {
    const [first] = await requireWorkspaces(context.queryClient)
    if (!first) throw redirect({ to: loginRoute.to })
    throw redirect({ to: overviewRoute.to, params: { slug: first.slug } })
  },
})
