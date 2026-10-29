import { createLazyFileRoute } from '@tanstack/react-router'

import { SignUpPage } from '../pages/auth/sign-up.tsx'

export const Route = createLazyFileRoute('/sign-up')({
  component: SignUpPage,
})
