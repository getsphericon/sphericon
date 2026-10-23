import { createLazyFileRoute } from '@tanstack/react-router'

import { TransactionalEmailsListPage } from '../../../../pages/transactional-emails/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/transactional-emails/')({
  component: TransactionalEmailsListPage,
})
