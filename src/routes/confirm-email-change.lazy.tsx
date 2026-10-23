import { createLazyFileRoute } from '@tanstack/react-router'

import { ConfirmEmailChangePage } from '../pages/auth/confirm-email-change.tsx'

export const Route = createLazyFileRoute('/confirm-email-change')({
  component: ConfirmEmailChangePage,
})
