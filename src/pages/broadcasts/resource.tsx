import {
  siteBroadcastsCreateMutation,
  siteBroadcastsGetOptions,
  siteBroadcastsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zCreateBroadcastInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { BroadcastForm } from './BroadcastForm.tsx'
import type { BroadcastValues } from './BroadcastForm.tsx'
import { DeliveryBlock } from './DeliveryBlock.tsx'

// A blank Segment or Integration is the whole list or the Workspace default.
const blankBroadcast: BroadcastValues = {
  name: '',
  subject: '',
  fromName: '',
  fromEmail: '',
  segmentId: '',
  integrationId: '',
  body: '',
  doc: null,
}

const broadcastSchema = resourceFormSchema(zCreateBroadcastInput, blankBroadcast)

const broadcastResource = defineResource({
  schema: broadcastSchema,
  Form: BroadcastForm,
  getOptions: siteBroadcastsGetOptions,
  createMutation: siteBroadcastsCreateMutation,
  updateMutation: siteBroadcastsUpdateMutation,
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
