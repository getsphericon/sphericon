import { createLazyFileRoute } from '@tanstack/react-router'

import { ForgotPasswordPage } from '../pages/auth/forgot-password.tsx'

export const Route = createLazyFileRoute('/forgot-password')({
  component: ForgotPasswordPage,
})
