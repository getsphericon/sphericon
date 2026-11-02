import * as z from 'zod'

// Loaded resources can omit edited fields or carry null for a cleared value. Fields outside
// the picked contract schema never round-trip into the request.
// A nested object (a Channel's content) can carry a null for a cleared value too.
export type ResourceValues<TPayload> = {
  [K in keyof TPayload]?: Loaded<TPayload[K]> | null | undefined
}

type Loaded<TValue> = TValue extends readonly unknown[]
  ? TValue
  : TValue extends object
    ? ResourceValues<TValue>
    : TValue

export interface ResourceFormSchema<TPayload, TValues extends object> {
  blank: TValues
  create: z.ZodType<TPayload, TValues>
  update: z.ZodType<TPayload, TValues>
  toValues: (resource: ResourceValues<TPayload>) => TValues
}

// Typed editor values meet the generated request schema here. Initial values describe the
// editor's empty state, including collections and incomplete structured drafts; only the
// generated schema decides whether a submitted value is valid.
export function resourceFormSchema<TSchema extends z.ZodObject, TValues extends object>(
  schema: TSchema,
  blank: TValues,
): ResourceFormSchema<z.output<TSchema>, TValues> {
  const fields = Object.keys(schema.shape)
  const empty = new Map<string, unknown>(Object.entries(blank))
  const clearedOnUpdate = (field: string) => {
    const shape = schema.shape[field]
    // Generated optional fields use exactOptional, whose isOptional() is false.
    if (!(shape instanceof z.ZodExactOptional)) {
      return undefined
    }
    if (shape.safeParse(null).success) {
      return null
    }
    return shape.safeParse('').success ? '' : undefined
  }
  const request = (update: boolean) =>
    z
      .custom<TValues>(
        (value) => typeof value === 'object' && value !== null && !Array.isArray(value),
      )
      .transform((values): Record<string, unknown> =>
        Object.fromEntries(
          Object.entries(values).flatMap(([field, value]) => {
            const shape = schema.shape[field]
            if (!shape) {
              return []
            }
            let supplied: unknown = value
            if (typeof value === 'string' && typeof empty.get(field) === 'string') {
              supplied = value.trim() || (update ? clearedOnUpdate(field) : undefined)
            } else if (
              value === null &&
              empty.get(field) === null &&
              !shape.safeParse(null).success
            ) {
              // A null empty state explicitly omits a nonnullable field (source-only documents).
              supplied = undefined
            }
            return supplied === undefined ? [] : [[field, supplied]]
          }),
        ),
      )
      .pipe(schema)

  return {
    blank,
    create: request(false),
    update: request(true),
    toValues: (resource) => {
      const loaded = new Map<string, unknown>(Object.entries(resource))
      return Object.assign(
        {},
        blank,
        Object.fromEntries(fields.map((field) => [field, loaded.get(field) ?? empty.get(field)])),
      )
    },
  }
}
