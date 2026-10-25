import * as z from 'zod'

import { conditionCodec, emptyGroup } from '../../components/conditions/condition.ts'
import {
  siteSegmentsCreateMutation,
  siteSegmentsGetOptions,
  siteSegmentsGetQueryKey,
  siteSegmentsListQueryKey,
  siteSegmentsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteCreateSegmentInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { SegmentForm } from './SegmentForm.tsx'

// A Segment is always a Condition: a stored segment without a condition loads as the empty
// Condition (matches all contacts); the contract has no null condition.

const segmentSchema = resourceFormSchema(
  z.object({
    name: zSiteCreateSegmentInput.shape.name,
    condition: conditionCodec.default(emptyGroup),
  }),
)

const segmentResource = defineResource({
  schema: segmentSchema,
  Form: SegmentForm,
  getOptions: siteSegmentsGetOptions,
  createMutation: siteSegmentsCreateMutation,
  updateMutation: siteSegmentsUpdateMutation,
  listQueryKey: siteSegmentsListQueryKey,
  getQueryKey: siteSegmentsGetQueryKey,
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
})

// Function declarations (hoisted) because the route files and this module import each other.
export function SegmentCreatePage() {
  return <segmentResource.CreatePage />
}

export function SegmentEditPage() {
  return <segmentResource.EditPage />
}
