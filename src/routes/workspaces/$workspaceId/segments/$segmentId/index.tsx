import { createFileRoute } from '@tanstack/react-router'

import { SegmentDetailPage } from '../../../../../pages/segments/detail.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/segments/$segmentId/')({
  component: SegmentDetailPage,
})
