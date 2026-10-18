import { createFileRoute } from '@tanstack/react-router'

import { SecurityPage } from '../../pages/account/security.tsx'

export const Route = createFileRoute('/account/security')({
  component: SecurityPage,
})
