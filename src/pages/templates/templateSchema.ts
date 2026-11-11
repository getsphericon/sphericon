import type { CreateTemplateInput } from '../../generated/site/types.gen.ts'
import { zCreateTemplateInput } from '../../generated/site/zod.gen.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { ResourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { TemplateValues } from './TemplateForm.tsx'

// An absent document keeps source-only content in the body editor.
const blankTemplate: TemplateValues = {
  name: '',
  subject: '',
  body: '',
  doc: null,
}

const flatSchema = resourceFormSchema(zCreateTemplateInput, blankTemplate)

// A message written in the editor is its document and the server builds the body; sending both
// is a 422.
function toBody(payload: CreateTemplateInput): CreateTemplateInput {
  const { doc, name, subject } = payload
  return doc ? { name, ...(subject !== undefined && { subject }), doc } : payload
}

export const templateSchema: ResourceFormSchema<CreateTemplateInput, TemplateValues> = {
  blank: flatSchema.blank,
  create: flatSchema.create.transform(toBody),
  update: flatSchema.update.transform(toBody),
  toValues: flatSchema.toValues,
}
