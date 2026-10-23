import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { requireWorkspaces } from '../../guards.ts'

export const Route = createFileRoute('/oauth/consent')({
  // /oauth/authorize (Go) redirects the browser here with the MCP client's authorization
  // request. Signed-out users sign in first and come back.
  validateSearch: z.object({
    client_id: z.string().catch(''),
    redirect_uri: z.string().catch(''),
    code_challenge: z.string().catch(''),
    state: z.string().catch(''),
    scope: z.string().catch(''),
  }),
  beforeLoad: async ({ context, location }) => {
    await requireWorkspaces(context.queryClient, location.href)
  },
})
