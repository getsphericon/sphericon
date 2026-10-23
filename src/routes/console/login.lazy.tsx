import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleLoginPage } from '../../console/login.tsx'

export const Route = createLazyFileRoute('/console/login')({
  component: ConsoleLoginPage,
})
