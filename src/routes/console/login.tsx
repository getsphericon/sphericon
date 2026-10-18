import { createFileRoute } from '@tanstack/react-router'

import { ConsoleLoginPage } from '../../console/login.tsx'

export const Route = createFileRoute('/console/login')({
  component: ConsoleLoginPage,
})
