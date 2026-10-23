import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/account/delete')({
  validateSearch: z.object({ token: z.string().catch('') }),
})
