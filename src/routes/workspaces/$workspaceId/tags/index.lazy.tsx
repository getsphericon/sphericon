import { createLazyFileRoute } from '@tanstack/react-router'

import { TagsListPage } from '../../../../pages/tags/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/tags/')({
  component: TagsListPage,
})
