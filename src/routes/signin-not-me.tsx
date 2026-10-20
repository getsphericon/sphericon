import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { SigninNotMePage } from '../pages/auth/signin-not-me.tsx'

export const Route = createFileRoute('/signin-not-me')({
  validateSearch: z.object({ token: z.string().catch('') }),
  component: SigninNotMePage,
})
