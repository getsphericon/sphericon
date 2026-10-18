import { createFileRoute } from '@tanstack/react-router'

import { RegisterPage } from '../pages/auth/register.tsx'

export const Route = createFileRoute('/register')({
  component: RegisterPage,
})
