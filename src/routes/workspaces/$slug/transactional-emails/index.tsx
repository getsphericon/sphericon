import { createFileRoute } from '@tanstack/react-router'

import { TransactionalEmailsListPage } from '../../../../pages/transactional-emails/list.tsx'

export const Route = createFileRoute('/workspaces/$slug/transactional-emails/')({
  component: TransactionalEmailsListPage,
})
