import { createLazyFileRoute } from '@tanstack/react-router'

import { ConfirmUserDeletionPage } from '../pages/account/confirm-user-deletion.tsx'

export const Route = createLazyFileRoute('/delete-account')({
  component: ConfirmUserDeletionPage,
})
