import { createLazyFileRoute } from '@tanstack/react-router'

import { BrandKitPage } from '../../../pages/brand-kit/page.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/brand-kit')({
  component: BrandKitPage,
})
