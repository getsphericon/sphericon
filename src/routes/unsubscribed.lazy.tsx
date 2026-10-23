import { createLazyFileRoute } from '@tanstack/react-router'

import { UnsubscribedPage } from '../pages/unsubscribed.tsx'

export const Route = createLazyFileRoute('/unsubscribed')({
  component: UnsubscribedPage,
})
