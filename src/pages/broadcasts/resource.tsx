import * as z from 'zod'

import {
  siteBroadcastsCreateMutation,
  siteBroadcastsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  siteBroadcastsSchedule,
  siteBroadcastsSend,
  siteBroadcastsUpdate,
} from '../../generated/site/sdk.gen.ts'
import type { CreateBroadcastInput, SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import {
  zBroadcastChannel,
  zCreateBroadcastInput,
  zEmailBroadcastContentInput,
  zSmsBroadcastContentInput,
  zTelegramBroadcastContentInput,
} from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import type { ResourceFormSchema, ResourceValues } from '../../resources/resourceFormSchema.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { BroadcastForm } from './BroadcastForm.tsx'
import type { BroadcastValues } from './BroadcastForm.tsx'
import { DeliveryBlock } from './DeliveryBlock.tsx'

// A blank Segment or Integration is the whole list or the Workspace default.
const blankBroadcast: BroadcastValues = {
  name: '',
  channel: '',
  subject: '',
  fromName: '',
  fromEmail: '',
  segmentId: '',
  integrationId: '',
  body: '',
  doc: null,
  text: '',
  imageUrl: '',
  buttons: [],
  linkPreview: true,
  unsubscribeButton: '',
  linkTracking: true,
}

// The form edits a Broadcast flat: its name, Segment and Integration beside its Channel's content
// fields. A blank Channel is not chosen yet.
const flatSchema = resourceFormSchema(
  zCreateBroadcastInput.pick({ name: true, segmentId: true, integrationId: true }).extend({
    channel: zBroadcastChannel,
    ...zEmailBroadcastContentInput.pick({
      subject: true,
      fromName: true,
      fromEmail: true,
      body: true,
      doc: true,
    }).shape,
    ...zTelegramBroadcastContentInput.pick({
      text: true,
      imageUrl: true,
      buttons: true,
      linkPreview: true,
      unsubscribeButton: true,
    }).shape,
    ...zSmsBroadcastContentInput.pick({ linkTracking: true }).shape,
  }),
  blankBroadcast,
)

type FlatBroadcast = z.output<typeof flatSchema.create>

// The request body nests the content under its Channel (ADR 0078).
function toBody({ name, segmentId, integrationId, ...content }: FlatBroadcast) {
  return {
    name,
    ...(segmentId !== undefined && { segmentId }),
    ...(integrationId !== undefined && { integrationId }),
    content: contentOf(content),
  } satisfies CreateBroadcastInput
}

// The content fields of the form, in the shape of the Broadcast's Channel. A message written in the
// email editor is its document and the server builds the body; sending both is a 422.
function contentOf({
  channel,
  doc,
  body,
  text,
  imageUrl,
  buttons,
  linkPreview,
  unsubscribeButton,
  linkTracking,
  ...email
}: Omit<FlatBroadcast, 'name' | 'segmentId' | 'integrationId'>): CreateBroadcastInput['content'] {
  if (channel === 'telegram') {
    return {
      channel,
      ...(text !== undefined && { text }),
      ...(imageUrl !== undefined && { imageUrl }),
      ...(buttons !== undefined && { buttons }),
      ...(linkPreview !== undefined && { linkPreview }),
      ...(unsubscribeButton !== undefined && { unsubscribeButton }),
    }
  }
  if (channel === 'sms') {
    return {
      channel,
      ...(text !== undefined && { text }),
      ...(linkTracking !== undefined && { linkTracking }),
    }
  }
  return { channel, ...email, ...(doc ? { doc } : body !== undefined && { body }) }
}

// A loaded Broadcast read back flat: its content's fields beside the rest.
function flatValues({ content, ...rest }: ResourceValues<CreateBroadcastInput>) {
  return flatSchema.toValues({ ...rest, ...(typeof content === 'object' ? content : {}) })
}

const broadcastSchema: ResourceFormSchema<CreateBroadcastInput, BroadcastValues> = {
  blank: flatSchema.blank,
  create: flatSchema.create.transform(toBody),
  update: flatSchema.update.transform(toBody),
  toValues: flatValues,
}

// Send now and Schedule deliver the saved content of a draft: the form holds no unsaved edits.
function deliverable({ resource, dirty }: { resource: SiteBroadcastResource; dirty: boolean }) {
  return resource.status === 'draft' && !dirty
}

const broadcastResource = defineResource({
  schema: broadcastSchema,
  Form: BroadcastForm,
  getOptions: siteBroadcastsGetOptions,
  createMutation: siteBroadcastsCreateMutation,
  update: siteBroadcastsUpdate,
  // Delivery of the saved draft, serialized with Save by the editor session.
  operations: {
    SEND: {
      request: async ({ subject: { path } }) => {
        const { data } = await siteBroadcastsSend({ path, throwOnError: true })
        return data
      },
      available: deliverable,
      texts: (t) => ({
        accepted: t(($) => $.notifications.broadcastSent),
        failedTitle: t(($) => $.alerts.broadcastSendErrorTitle),
      }),
    },
    SCHEDULE: {
      request: async ({ subject: { path }, input }) => {
        const { scheduledAt } = input
        if (scheduledAt === undefined) {
          throw new Error('Schedule was started without a time')
        }
        const { data } = await siteBroadcastsSchedule({
          path,
          body: { scheduledAt },
          throwOnError: true,
        })
        return data
      },
      available: deliverable,
      texts: (t) => ({
        accepted: t(($) => $.notifications.broadcastScheduled),
        failedTitle: t(($) => $.alerts.broadcastSendErrorTitle),
      }),
    },
  },
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
