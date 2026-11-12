import { schemaResolver, useForm } from '@mantine/form'
import { useId } from 'react'
import * as z from 'zod'

import { getFieldErrors } from '../utils/apiErrors.ts'

// The messages a problem lists by field (its `errors` member).
type FieldErrors = NonNullable<ReturnType<typeof getFieldErrors>>

// The form useSchemaForm returns. @mantine/form's own form type is not importable (its .d.mts
// re-exports a missing file), so it is derived from the hook; the members typed by the
// validation rules are left out, they are the hook's business.
export type SchemaForm<TValues extends object, TOutput> = Omit<
  ReturnType<typeof useForm<TValues, TOutput>>,
  'validate' | 'validateField' | 'isValid'
> & {
  showFieldErrors: (error: unknown) => FieldErrors | undefined
}

// useSchemaForm is a form validated by a zod schema built from the generated (TypeSpec) ones, so
// the contract is the one source of its rules; a submit hands over the schema's output. The
// messages come from the zod error map in src/i18n.ts.
//
// Validation follows docs/standards/ui.md: a field is not validated when the form opens, it is
// validated when it loses focus, an error that shows is re-checked on each keystroke, and a failed
// submit focuses the first invalid field in the page's order.
//
// A refusal from the server follows the same rules (docs/standards/ui.md, Errors): showFieldErrors
// puts each message of the problem's `errors` under the field the form renders for its path, and
// focuses the first of them in the page's order. It hands back what it could not place: the
// messages of paths the form does not render, or an empty object when the problem names no field,
// so the page decides how they show. Undefined means every refusal shows under its field.
export function useSchemaForm<TValues extends object, TOutput>(
  schema: z.ZodType<TOutput, TValues>,
  initialValues: TValues,
): SchemaForm<TValues, TOutput> {
  // A name of its own scopes the form's fields (their data-path) to it, so finding an invalid
  // field never lands on another form's field with the same path. Mantine takes letters, digits and
  // dashes only, so each other character of the React id becomes a dash, which keeps ids apart.
  const name = `form${useId().replaceAll(/[^a-zA-Z0-9-]/g, '-')}`
  const form = useForm<TValues, TOutput>({
    name,
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

  const showFieldErrors = (error: unknown) => {
    const fieldErrors = getFieldErrors(error) ?? {}
    const placed = Object.keys(fieldErrors).filter(
      (path) => fieldErrors[path]?.[0] && form.getInputNode(path),
    )
    for (const path of placed) {
      form.setFieldError(path, fieldErrors[path]?.[0])
    }
    focusFirstInvalid(form.getInputNode, placed)
    const unplaced = Object.fromEntries(
      Object.entries(fieldErrors).filter(([path]) => !placed.includes(path)),
    )
    return placed.length > 0 && Object.keys(unplaced).length === 0 ? undefined : unplaced
  }

  return { ...form, onSubmit, showFieldErrors }
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
