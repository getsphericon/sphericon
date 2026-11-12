import * as z from 'zod'

import { based } from '../../editing/editorSession.ts'
import { siteBroadcastsSchedule, siteBroadcastsSend } from '../../generated/site/sdk.gen.ts'
import type { CreateBroadcastInput, SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import {
  zBroadcastChannel,
  zCreateBroadcastInput,
  zEmailBroadcastContentInput,
  zSiteBroadcastsScheduleBody,
  zSmsBroadcastContentInput,
  zTelegramBroadcastContentInput,
} from '../../generated/site/zod.gen.ts'
import type { ResourceEditOperation } from '../../resources/defineResource.tsx'
import type { ResourceFormSchema, ResourceValues } from '../../resources/resourceFormSchema.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { BroadcastValues } from './BroadcastForm.tsx'

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

export const broadcastSchema: ResourceFormSchema<CreateBroadcastInput, BroadcastValues> = {
  blank: flatSchema.blank,
  create: flatSchema.create.transform(toBody),
  update: flatSchema.update.transform(toBody),
  toValues: flatValues,
}

// Send now and Schedule deliver the saved content of a draft: the form holds no unsaved edits.
function deliverable({ resource, dirty }: { resource: SiteBroadcastResource; dirty: boolean }) {
  return resource.status === 'draft' && !dirty
}

// Delivery of the saved draft, serialized with Save by the editor session. Each names the
// revision the editor shows (ADR 0119), so a draft changed since is refused and not sent.
export const broadcastOperations = {
  SEND: {
    request: async ({ subject: { path }, revision }) => {
      const { data } = await siteBroadcastsSend({
        path,
        body: { revision: based(revision) },
        throwOnError: true,
      })
      return data
    },
    available: deliverable,
    texts: (t) => ({
      accepted: t(($) => $.notifications.broadcastSent),
      failedTitle: t(($) => $.alerts.broadcastSendErrorTitle),
    }),
  },
  SCHEDULE: {
    // Its input is the request's body: the time DeliveryBlock schedules at.
    request: async ({ subject: { path }, input, revision }) => {
      const { scheduledAt } = zSiteBroadcastsScheduleBody.omit({ revision: true }).parse(input)
      const { data } = await siteBroadcastsSchedule({
        path,
        body: { scheduledAt, revision: based(revision) },
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
} satisfies Record<string, ResourceEditOperation<SiteBroadcastResource, BroadcastValues>>
