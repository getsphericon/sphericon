import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleInstancePage } from '../../../console/instance.tsx'

export const Route = createLazyFileRoute('/console/_authed/instance')({
  component: ConsoleInstancePage,
})
