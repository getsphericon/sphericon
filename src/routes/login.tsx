import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteSigninError } from '../generated/site/zod.gen.ts'
import { LoginPage } from '../pages/auth/login.tsx'

export const Route = createFileRoute('/login')({
  validateSearch: z.object({
    // A same-origin path to return to after signing in.
    redirect: z
      .string()
      .startsWith('/')
      .refine((path) => !path.startsWith('//'))
      .optional()
      .catch(undefined),
    // Why a sign-in with a provider did not start a session (set by the server's callback).
    error: zSiteSigninError.optional().catch(undefined),
    // The Second factor challenge of a provider sign-in (set by the server's callback).
    challenge: z.string().optional().catch(undefined),
  }),
  component: LoginPage,
})
