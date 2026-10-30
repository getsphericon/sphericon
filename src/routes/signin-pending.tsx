import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zReturnPath } from '../utils/returnPath.ts'

// The screen a Sign-in provider's sign-up lands on while the Instance asks Sign-up items (ADR
// 0062), set by the server's callback.
export const Route = createFileRoute('/signin-pending')({
  validateSearch: z.object({
    // The signed reference of the pending sign-up.
    token: z.string().catch(''),
    // Where the provider sign-in began, to go on to once the sign-up is complete.
    redirect: zReturnPath,
  }),
})
