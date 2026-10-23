import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/confirm-email-change')({
  validateSearch: z.object({ token: z.string().catch('') }),
})
