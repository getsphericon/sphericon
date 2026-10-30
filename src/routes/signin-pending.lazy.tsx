import { createLazyFileRoute } from '@tanstack/react-router'

import { SigninPendingPage } from '../pages/auth/signin-pending.tsx'

export const Route = createLazyFileRoute('/signin-pending')({
  component: SigninPendingPage,
})
