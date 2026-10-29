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
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { docCodec } from '../../utils/messageDoc.ts'
import { BroadcastForm } from './BroadcastForm.tsx'
import { DeliveryBlock } from './DeliveryBlock.tsx'

// The name is required on both create and update; a blank Segment or Integration is the whole
// list or the Workspace default. The document is held as JSON text; a blank one is absent,
// so a source-only save writes a body.
const broadcastSchema = resourceFormSchema(
  zSiteCreateBroadcastInput
    .pick({
      subject: true,
      fromName: true,
      fromEmail: true,
      segmentId: true,
      integrationId: true,
      body: true,
    })
    .extend({ name: z.string().min(1), doc: z.exactOptional(docCodec) }),
)

const broadcastResource = defineResource({
  schema: broadcastSchema,
  Form: BroadcastForm,
  getOptions: siteBroadcastsGetOptions,
  createMutation: siteBroadcastsCreateMutation,
  updateMutation: siteBroadcastsUpdateMutation,
  listQueryKey: siteBroadcastsListQueryKey,
  getQueryKey: siteBroadcastsGetQueryKey,
  editRoute: broadcastsEditRoute,
  idParam: 'broadcastId',
  texts: (t) => ({
    createTitle: t(($) => $.broadcasts.createTitle),
    editTitle: t(($) => $.broadcasts.editTitle),
    created: t(($) => $.notifications.broadcastCreated),
    updated: t(($) => $.notifications.broadcastUpdated),
    loadErrorTitle: t(($) => $.alerts.broadcastLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.broadcastSaveErrorTitle),
  }),
  editTarget: (created, { workspaceId }) => ({
    to: broadcastsEditRoute.to,
    params: { workspaceId, broadcastId: created.id },
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
