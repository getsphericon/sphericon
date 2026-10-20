import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { ConfirmUserDeletionPage } from '../../pages/account/confirm-user-deletion.tsx'

export const Route = createFileRoute('/account/delete')({
  validateSearch: z.object({ token: z.string().catch('') }),
  component: ConfirmUserDeletionPage,
})
