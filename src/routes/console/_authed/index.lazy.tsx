import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleHomePage } from '../../../console/home.tsx'

export const Route = createLazyFileRoute('/console/_authed/')({
  component: ConsoleHomePage,
})
