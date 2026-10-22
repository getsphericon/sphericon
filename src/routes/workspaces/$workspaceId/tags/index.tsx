import { createFileRoute } from '@tanstack/react-router'

import { TagsListPage } from '../../../../pages/tags/list.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/tags/')({
  component: TagsListPage,
})
