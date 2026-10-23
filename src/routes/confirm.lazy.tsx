import { createLazyFileRoute } from '@tanstack/react-router'

import { ConfirmSubscriptionPage } from '../pages/confirm.tsx'

export const Route = createLazyFileRoute('/confirm')({
  component: ConfirmSubscriptionPage,
})
