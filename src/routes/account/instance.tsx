import { createFileRoute } from '@tanstack/react-router'

import { InstancePage } from '../../pages/account/instance.tsx'

export const Route = createFileRoute('/account/instance')({
  component: InstancePage,
})
