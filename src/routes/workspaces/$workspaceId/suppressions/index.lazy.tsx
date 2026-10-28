import { createLazyFileRoute } from '@tanstack/react-router'

import { SuppressionsListPage } from '../../../../pages/suppressions/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/suppressions/')({
  component: SuppressionsListPage,
})
