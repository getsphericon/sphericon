import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { httpUrlParam } from '../searchSchemas.ts'

export const Route = createFileRoute('/unsubscribed')({
  validateSearch: z.object({ all: httpUrlParam }),
})
