import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleSignupItemsPage } from '../../../console/signupItems.tsx'

export const Route = createLazyFileRoute('/console/_authed/signup-items/')({
  component: ConsoleSignupItemsPage,
})
