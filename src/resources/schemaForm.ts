import { schemaResolver, useForm } from '@mantine/form'
import * as z from 'zod'

// The form useSchemaForm returns. @mantine/form's own form type is not importable (its .d.mts
// re-exports a missing file), so it is derived from the hook; the members typed by the
// validation rules are left out, they are the hook's business.
export type SchemaForm<TValues extends Record<string, unknown>, TOutput> = Omit<
  ReturnType<typeof useForm<TValues, TOutput>>,
  'validate' | 'validateField' | 'isValid'
>

// useSchemaForm is a form validated by a zod schema built from the generated (TypeSpec) ones, so
// the contract is the one source of its rules; a submit hands over the schema's output. The
// messages come from the zod error map in src/i18n.ts.
export function useSchemaForm<TValues extends Record<string, unknown>, TOutput>(
  schema: z.ZodType<TOutput, TValues>,
  initialValues: TValues,
): SchemaForm<TValues, TOutput> {
  return useForm<TValues, TOutput>({
    initialValues,
    validate: schemaResolver(schema, { sync: true }),
    transformValues: (values) => schema.parse(values),
  })
}

// trimmed reads a contract string field from a text input: the spaces around what was typed are
// not part of the value, so a blank input is an empty value the field's own rules judge.
export function trimmed<TOutput>(field: z.ZodType<TOutput, string>) {
  return z.string().trim().pipe(field)
}
