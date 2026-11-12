import {
  siteSegmentsCreateMutation,
  siteSegmentsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteSegmentsUpdate } from '../../generated/site/sdk.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { withRevision } from '../../resources/resourceEditor.ts'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { Route as segmentsDetailRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/index.tsx'
import { SegmentForm } from './SegmentForm.tsx'
import { segmentSchema } from './segmentSchema.ts'

const segmentResource = defineResource({
  schema: segmentSchema,
  Form: SegmentForm,
  getOptions: siteSegmentsGetOptions,
  createMutation: siteSegmentsCreateMutation,
  update: withRevision(siteSegmentsUpdate),
  operations: {},
  editRoute: segmentsEditRoute,
  idParam: 'segmentId',
  texts: (t) => ({
    createTitle: t(($) => $.segments.createTitle),
    editTitle: t(($) => $.segments.editTitle),
    created: t(($) => $.notifications.segmentCreated),
    updated: t(($) => $.notifications.segmentUpdated),
    loadErrorTitle: t(($) => $.alerts.segmentLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.segmentSaveErrorTitle),
  }),
  editTarget: (created, { workspaceId }) => ({
    to: segmentsEditRoute.to,
    params: { workspaceId, segmentId: created.id },
  }),
  readOnlyTarget: (loaded, { workspaceId, id }) =>
    loaded.archivedAt
      ? { to: segmentsDetailRoute.to, params: { workspaceId, segmentId: id } }
      : undefined,
})

// Function declarations (hoisted) because the route files and this module import each other.
export function SegmentCreatePage() {
  return <segmentResource.CreatePage />
}

export function SegmentEditPage() {
  return <segmentResource.EditPage />
}
