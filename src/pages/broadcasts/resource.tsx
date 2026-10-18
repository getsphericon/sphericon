import * as z from 'zod'

import {
  siteBroadcastsCreateMutation,
  siteBroadcastsGetOptions,
  siteBroadcastsGetQueryKey,
  siteBroadcastsListQueryKey,
  siteBroadcastsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteCreateBroadcastInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/edit.tsx'
import { BroadcastForm } from './BroadcastForm.tsx'
import { DeliveryBlock } from './DeliveryBlock.tsx'

// The integration is not edited here, so it never round-trips into the payload. The name is
// required on both create and update.
const broadcastSchema = resourceFormSchema(
  zSiteCreateBroadcastInput
    .pick({
      subject: true,
      fromName: true,
      fromEmail: true,
      segmentId: true,
      body: true,
    })
    .extend({ name: z.string().min(1) }),
)

const broadcastResource = defineResource({
  schema: broadcastSchema,
  Form: BroadcastForm,
  getOptions: siteBroadcastsGetOptions,
  createMutation: siteBroadcastsCreateMutation,
  updateMutation: siteBroadcastsUpdateMutation,
  listQueryKey: siteBroadcastsListQueryKey,
  getQueryKey: siteBroadcastsGetQueryKey,
  idParam: 'broadcastId',
  texts: (t) => ({
    createTitle: t(($) => $.broadcasts.createTitle),
    editTitle: t(($) => $.broadcasts.editTitle),
    created: t(($) => $.notifications.broadcastCreated),
    updated: t(($) => $.notifications.broadcastUpdated),
    loadErrorTitle: t(($) => $.alerts.broadcastLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.broadcastSaveErrorTitle),
  }),
  editTarget: (created, { slug }) => ({
    to: broadcastsEditRoute.to,
    params: { slug, broadcastId: created.id },
  }),
})

// Function declarations (hoisted) because the route files and this module import each other.
export function BroadcastCreatePage() {
  return <broadcastResource.CreatePage />
}

export function BroadcastEditPage() {
  return (
    <broadcastResource.EditPage>
      {(broadcast) => <DeliveryBlock broadcast={broadcast} />}
    </broadcastResource.EditPage>
  )
}
