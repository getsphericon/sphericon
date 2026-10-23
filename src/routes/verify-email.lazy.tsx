import { createLazyFileRoute } from '@tanstack/react-router'

import { VerifyEmailPage } from '../pages/auth/verify-email.tsx'

export const Route = createLazyFileRoute('/verify-email')({
  component: VerifyEmailPage,
})
