import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { UnsubscribedPage } from '../pages/unsubscribed.tsx'

export const Route = createFileRoute('/unsubscribed')({
  validateSearch: z.object({ all: z.string().optional() }),
  component: UnsubscribedPage,
})
