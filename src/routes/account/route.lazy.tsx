import { createLazyFileRoute } from '@tanstack/react-router'

import { AccountLayout } from '../../layouts/AccountLayout.tsx'

export const Route = createLazyFileRoute('/account')({
  component: AccountLayout,
})
