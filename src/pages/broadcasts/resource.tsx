import {
  siteBroadcastsCreateMutation,
  siteBroadcastsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteBroadcastsUpdate } from '../../generated/site/sdk.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { withRevision } from '../../resources/resourceEditor.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { broadcastOperations, broadcastSchema } from './broadcastEditor.ts'
import { BroadcastForm } from './BroadcastForm.tsx'
import { DeliveryBlock } from './DeliveryBlock.tsx'

const broadcastResource = defineResource({
  schema: broadcastSchema,
  Form: BroadcastForm,
  getOptions: siteBroadcastsGetOptions,
  createMutation: siteBroadcastsCreateMutation,
  update: withRevision(siteBroadcastsUpdate),
  operations: broadcastOperations,
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
  readOnlyTarget: (loaded, { workspaceId, id }) =>
    loaded.archivedAt
      ? { to: broadcastsReportRoute.to, params: { workspaceId, broadcastId: id } }
      : undefined,
})

// Function declarations (hoisted) because the route files and this module import each other.
export function BroadcastCreatePage() {
  return <broadcastResource.CreatePage />
}

export function BroadcastEditPage() {
  return (
    <broadcastResource.EditPage>
      {(broadcast, editor) => <DeliveryBlock broadcast={broadcast} editor={editor} />}
    </broadcastResource.EditPage>
  )
}
