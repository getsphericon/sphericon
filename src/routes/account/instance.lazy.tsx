import { createLazyFileRoute } from '@tanstack/react-router'

import { InstancePage } from '../../pages/account/instance.tsx'

export const Route = createLazyFileRoute('/account/instance')({
  component: InstancePage,
})
