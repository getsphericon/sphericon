import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleLayout } from '../../../console/layout.tsx'

export const Route = createLazyFileRoute('/console/_authed')({
  component: ConsoleLayout,
})
