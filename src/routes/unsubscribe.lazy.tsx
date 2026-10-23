import { createLazyFileRoute } from '@tanstack/react-router'

import { UnsubscribePage } from '../pages/unsubscribe.tsx'

export const Route = createLazyFileRoute('/unsubscribe')({
  component: UnsubscribePage,
})
