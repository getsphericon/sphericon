import { schemaResolver, useForm } from '@mantine/form'
import * as z from 'zod'

// The form useSchemaForm returns. @mantine/form's own form type is not importable (its .d.mts
// re-exports a missing file), so it is derived from the hook; the members typed by the
// validation rules are left out, they are the hook's business.
export type SchemaForm<TValues extends object, TOutput> = Omit<
  ReturnType<typeof useForm<TValues, TOutput>>,
  'validate' | 'validateField' | 'isValid'
>

// useSchemaForm is a form validated by a zod schema built from the generated (TypeSpec) ones, so
// the contract is the one source of its rules; a submit hands over the schema's output. The
// messages come from the zod error map in src/i18n.ts.
//
// Validation follows docs/standards/ui.md: a field is not validated when the form opens, it is
// validated when it loses focus, an error that shows is re-checked on each keystroke, and a failed
// submit focuses the first invalid field in the page's order.
export function useSchemaForm<TValues extends object, TOutput>(
  schema: z.ZodType<TOutput, TValues>,
  initialValues: TValues,
): SchemaForm<TValues, TOutput> {
  const form = useForm<TValues, TOutput>({
    initialValues,
    validate: schemaResolver(schema, { sync: true }),
    transformValues: (values) => schema.parse(values),
    validateInputOnBlur: true,
    enhanceGetInputProps: ({ inputProps, field, form: renderedForm }) => {
      // The error showing at render time is the one the keystroke is about to clear.
      if (!renderedForm.errors[field]) {
        return undefined
      }
      const { onChange } = inputProps
      return {
        onChange: (...args: unknown[]) => {
          onChange(...args)
          renderedForm.validateField(field)
        },
      }
    },
  })

  const onSubmit: typeof form.onSubmit = (handleSubmit, handleValidationFailure) =>
    form.onSubmit(handleSubmit, (errors, values, event) => {
      focusFirstInvalid(form.getInputNode, Object.keys(errors))
      handleValidationFailure?.(errors, values, event)
    })

  return { ...form, onSubmit }
}

// focusFirstInvalid focuses the field, among the invalid ones, that comes first in the document:
// the order of the schema's keys is not the order of the page.
function focusFirstInvalid(
  getInputNode: (path: string) => HTMLElement | null,
  paths: string[],
): void {
  const nodes = paths.flatMap((path) => getInputNode(path) ?? [])
  nodes
    .toSorted((a, b) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1))
    .at(0)
    ?.focus()
}

// trimmed reads a contract string field from a text input: the spaces around what was typed are
// not part of the value, so a blank input is an empty value the field's own rules judge.
export function trimmed<TOutput>(field: z.ZodType<TOutput, string>) {
  return z.string().trim().pipe(field)
}
