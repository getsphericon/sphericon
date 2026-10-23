import { createLazyFileRoute } from '@tanstack/react-router'

import { RegisterPage } from '../pages/auth/register.tsx'

export const Route = createLazyFileRoute('/register')({
  component: RegisterPage,
})
