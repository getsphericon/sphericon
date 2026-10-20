import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteSigninError } from '../../generated/site/zod.gen.ts'
import { SecurityPage } from '../../pages/account/security.tsx'

export const Route = createFileRoute('/account/security')({
  validateSearch: z.object({
    // Why linking a provider account did not succeed (set by the server's callback).
    error: zSiteSigninError.optional().catch(undefined),
  }),
  component: SecurityPage,
})
