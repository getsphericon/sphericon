import { createLazyFileRoute } from '@tanstack/react-router'

import { SegmentDetailPage } from '../../../../../pages/segments/detail.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/segments/$segmentId/')({
  component: SegmentDetailPage,
})
