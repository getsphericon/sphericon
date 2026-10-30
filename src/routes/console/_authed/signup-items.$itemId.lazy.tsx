import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleSignupItemPage } from '../../../console/signupItem.tsx'

export const Route = createLazyFileRoute('/console/_authed/signup-items/$itemId')({
  component: ConsoleSignupItemPage,
})
