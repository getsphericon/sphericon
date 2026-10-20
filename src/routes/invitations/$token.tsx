import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteSigninError } from '../../generated/site/zod.gen.ts'
import { AcceptInvitationPage } from '../../pages/invitations/accept.tsx'

export const Route = createFileRoute('/invitations/$token')({
  validateSearch: z.object({
    // Why a provider sign-in did not accept the invitation (set by the server's callback).
    error: zSiteSigninError.optional().catch(undefined),
  }),
  component: AcceptInvitationPage,
})
