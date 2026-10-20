import { createFileRoute } from '@tanstack/react-router'

import { GeneralPage } from '../../../../pages/workspace/settings/general.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/')({
  component: GeneralPage,
})
