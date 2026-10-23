import { createLazyFileRoute } from '@tanstack/react-router'

import { ResetPasswordPage } from '../pages/auth/reset-password.tsx'

export const Route = createLazyFileRoute('/reset-password')({
  component: ResetPasswordPage,
})
