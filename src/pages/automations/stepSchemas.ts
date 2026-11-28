import * as z from 'zod'

import {
  zAutomationHttpHeader,
  zEditorDocument,
  zEmailStepInput,
  zSigningSecret,
  zStepName,
  zTelegramButton,
} from '../../generated/site/zod.gen.ts'
import { i18n } from '../../i18n.ts'
import type { EditedKind, StepDrafts } from './definition.ts'

// The rules each Step's form checks, on blur and on Save (docs/standards/ui.md), over the draft
// the form holds: what the contract refuses, said under the field before the server says it.
// Each schema's output is the draft itself: the flow holds what the form holds.

const integrationId = z.string()

export const stepSchemas = {
  email: zEmailStepInput.pick({ subject: true }).extend({
    integrationId,
    body: z.string(),
    doc: zEditorDocument.nullable(),
  }),
  telegram: z.object({
    integrationId,
    text: z.string(),
    unsubscribeButton: z.string(),
    imageUrl: z.string(),
    buttons: z.array(zTelegramButton),
    linkPreview: z.boolean(),
  }),
  sms: z.object({ integrationId, text: z.string(), linkTracking: z.boolean() }),
  // A stored step keeps its URL when the field is left empty; a new one needs one. An empty secret
  // keeps the stored one, or sets none. Its name is the form later Steps read its response by.
  http: z
    .object({
      stored: z.boolean(),
      name: z.string(),
      url: z.string(),
      headers: z.array(zAutomationHttpHeader),
      body: z.string(),
      secret: z.union([z.literal(''), zSigningSecret]),
    })
    .superRefine((draft, context) => {
      // An empty input is a missing value: the zod error map (src/i18n.ts) says so.
      if (draft.name === '') {
        context.addIssue({ code: 'custom', path: ['name'], input: '' })
      } else if (!zStepName.safeParse(draft.name).success) {
        context.addIssue({
          code: 'custom',
          path: ['name'],
          message: i18n.t(($) => $.automations.httpNameFormat),
        })
      }
      if (!draft.stored && draft.url.trim() === '') {
        context.addIssue({ code: 'custom', path: ['url'], input: '' })
      }
    }),
  wait: z.object({ seconds: z.union([z.literal(''), z.number()]).pipe(z.int().min(1)) }),
  apply_tag: z.object({ tag: z.string().trim().min(1) }),
  remove_tag: z.object({ tag: z.string().trim().min(1) }),
  // A Wait until's form holds the form of its moment (MomentForms.tsx), whose rules are momentRules'.
} satisfies { [K in Exclude<EditedKind, 'wait_until'>]: z.ZodType<unknown, StepDrafts[K]> }
