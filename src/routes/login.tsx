import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { LoginPage } from '../pages/auth/login.tsx'

export const Route = createFileRoute('/login')({
  // `redirect` is a same-origin path to return to after signing in.
  validateSearch: z.object({
    redirect: z
      .string()
      .startsWith('/')
      .refine((path) => !path.startsWith('//'))
      .optional()
      .catch(undefined),
  }),
  component: LoginPage,
})
