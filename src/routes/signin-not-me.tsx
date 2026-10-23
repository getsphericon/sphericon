import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/signin-not-me')({
  validateSearch: z.object({ token: z.string().catch('') }),
})
