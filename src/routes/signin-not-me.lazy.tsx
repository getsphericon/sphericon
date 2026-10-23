import { createLazyFileRoute } from '@tanstack/react-router'

import { SigninNotMePage } from '../pages/auth/signin-not-me.tsx'

export const Route = createLazyFileRoute('/signin-not-me')({
  component: SigninNotMePage,
})
