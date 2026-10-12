import * as z from 'zod'

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
import { segmentsEditRoute } from '../../router.tsx'
import { definitionCodec, emptyGroup } from './rules.ts'
import { SegmentForm } from './SegmentForm.tsx'

// A Segment is always a rule: a stored segment without a definition loads as the empty
// rule group (matches all contacts); the contract has no null definition.

const segmentSchema = resourceFormSchema(
  z.object({
    name: zSiteCreateSegmentInput.shape.name,
    definition: definitionCodec.default(emptyGroup),
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
  idParam: 'segmentId',
  texts: (t) => ({
    createTitle: t(($) => $.segments.createTitle),
    editTitle: t(($) => $.segments.editTitle),
    created: t(($) => $.notifications.segmentCreated),
    updated: t(($) => $.notifications.segmentUpdated),
    loadErrorTitle: t(($) => $.alerts.segmentLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.segmentSaveErrorTitle),
  }),
  editTarget: (created, { slug }) => ({
    to: segmentsEditRoute.to,
    params: { slug, segmentId: created.id },
  }),
})

// Function declarations (hoisted) because router.tsx and this module import each other.
export function SegmentCreatePage() {
  return <segmentResource.CreatePage />
}

export function SegmentEditPage() {
  return <segmentResource.EditPage />
}
