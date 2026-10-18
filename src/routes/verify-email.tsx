import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { VerifyEmailPage } from '../pages/auth/verify-email.tsx'

export const Route = createFileRoute('/verify-email')({
  validateSearch: z.object({ token: z.string().catch('') }),
  component: VerifyEmailPage,
})
