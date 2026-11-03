import * as z from 'zod'

import {
  siteBroadcastsCreateMutation,
  siteBroadcastsGetOptions,
  siteBroadcastsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { CreateBroadcastInput } from '../../generated/site/types.gen.ts'
import {
  zBroadcastChannel,
  zCreateBroadcastInput,
  zEmailBroadcastContentInput,
  zTelegramBroadcastContentInput,
} from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import type { ResourceFormSchema, ResourceValues } from '../../resources/resourceFormSchema.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
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
  }),
  blankBroadcast,
)

type FlatBroadcast = z.output<typeof flatSchema.create>

// The request body nests the content under its Channel (ADR 0078). A message written in the
// editor is its document and the server builds the body; sending both is a 422.
function toBody({
  name,
  segmentId,
  integrationId,
  channel,
  doc,
  body,
  text,
  imageUrl,
  buttons,
  linkPreview,
  unsubscribeButton,
  ...email
}: FlatBroadcast) {
  return {
    name,
    ...(segmentId !== undefined && { segmentId }),
    ...(integrationId !== undefined && { integrationId }),
    content:
      channel === 'telegram'
        ? {
            channel,
            ...(text !== undefined && { text }),
            ...(imageUrl !== undefined && { imageUrl }),
            ...(buttons !== undefined && { buttons }),
            ...(linkPreview !== undefined && { linkPreview }),
            ...(unsubscribeButton !== undefined && { unsubscribeButton }),
          }
        : { channel, ...email, ...(doc ? { doc } : body !== undefined && { body }) },
  } satisfies CreateBroadcastInput
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
