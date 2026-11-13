import { createLazyFileRoute } from '@tanstack/react-router'

import { LinkExpiredPage } from '../pages/link-expired.tsx'

export const Route = createLazyFileRoute('/link-expired')({
  component: LinkExpiredPage,
})
