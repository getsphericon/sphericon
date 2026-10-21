import { createFileRoute } from '@tanstack/react-router'

import { BrandKitPage } from '../../../pages/brand-kit/page.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/brand-kit')({
  component: BrandKitPage,
})
