import { createLazyFileRoute } from '@tanstack/react-router'

import { LoginPage } from '../pages/auth/login.tsx'

export const Route = createLazyFileRoute('/login')({
  component: LoginPage,
})
