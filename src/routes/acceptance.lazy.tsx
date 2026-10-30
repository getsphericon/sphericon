import { createLazyFileRoute } from '@tanstack/react-router'

import { AcceptancePage } from '../pages/account/acceptance.tsx'

export const Route = createLazyFileRoute('/acceptance')({
  component: AcceptancePage,
})
