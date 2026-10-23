import { createLazyFileRoute } from '@tanstack/react-router'

import { CustomFieldsListPage } from '../../../../pages/custom-fields/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/custom-fields/')({
  component: CustomFieldsListPage,
})
