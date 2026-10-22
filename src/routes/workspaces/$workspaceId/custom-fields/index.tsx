import { createFileRoute } from '@tanstack/react-router'

import { CustomFieldsListPage } from '../../../../pages/custom-fields/list.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/custom-fields/')({
  component: CustomFieldsListPage,
})
