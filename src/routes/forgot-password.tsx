import { createFileRoute } from '@tanstack/react-router'

import { ForgotPasswordPage } from '../pages/auth/forgot-password.tsx'

export const Route = createFileRoute('/forgot-password')({
  component: ForgotPasswordPage,
})
