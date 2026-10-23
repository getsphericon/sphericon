import { createLazyFileRoute } from '@tanstack/react-router'

import { GeneralPage } from '../../../../pages/workspace/settings/general.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/')({
  component: GeneralPage,
})
