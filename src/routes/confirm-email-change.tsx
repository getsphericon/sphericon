import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { ConfirmEmailChangePage } from '../pages/auth/confirm-email-change.tsx'

export const Route = createFileRoute('/confirm-email-change')({
  validateSearch: z.object({ token: z.string().catch('') }),
  component: ConfirmEmailChangePage,
})
