import type { EditorAdapter, EditorOperation, EditorReceipt } from '../editing/editorSession.ts'
import type { ResourceFormSchema, ResourceValues } from './resourceFormSchema.ts'

interface ResourcePath {
  workspaceId: string
  id: string
}

export interface ResourceEditorSubject<TResource> {
  path: ResourcePath
  resource: TResource
}

// An operation of a resource's editor besides Save (Publish, Send now, …): it acts on the stored
// resource at `subject.path` and answers with it. One that `replaces` the form (Revert) gives the
// values the form then holds.
export type ResourceOperation<TResource, TValues> = EditorOperation<
  ResourceEditorSubject<TResource>,
  TResource,
  TValues
>

// The generated client call that saves an edit of a resource.
export type ResourceUpdate<TPayload, TResource> = (options: {
  path: ResourcePath
  body: TPayload
  throwOnError: true
}) => Promise<{ data: TResource }>

export interface ResourceSubmission<TPayload> {
  body: TPayload
  fingerprint: string
}

// The form's values as text, whatever their keys' order; invalid values have one too.
function valuesFingerprint(values: object): string {
  return JSON.stringify(values, (_key, value: unknown) =>
    value !== null && typeof value === 'object' && !Array.isArray(value)
      ? Object.fromEntries(Object.entries(value).toSorted(([a], [b]) => a.localeCompare(b)))
      : value,
  )
}

// The schema-form adapter of the editor session (src/editing/editorSession.ts): the content is the
// form's values, Save sends what the update schema makes of them. On the acknowledgement a form
// still holding the submission takes the response's normalized values and is clean; a form edited
// meanwhile keeps its values, and the submission becomes the acknowledged content. Its pieces are
// those of the resource's description (defineResource).
export function resourceEditorAdapter<
  TPayload,
  TValues extends object,
  TResource extends ResourceValues<TPayload>,
  TOperation extends string,
>({
  id,
  schema,
  editedValues,
  update,
  operations,
}: {
  id: string
  schema: ResourceFormSchema<TPayload, TValues>
  // The edited values of a resource whose fields are not at its top level.
  editedValues?: ((resource: TResource) => ResourceValues<TPayload>) | undefined
  // The generated client call that saves an edit.
  update: ResourceUpdate<TPayload, TResource>
  operations: Record<TOperation, ResourceOperation<TResource, TValues>>
}): EditorAdapter<
  ResourceEditorSubject<TResource>,
  TResource,
  TValues,
  ResourceSubmission<TPayload>,
  EditorReceipt<TResource>,
  TOperation
> {
  const values = (resource: TResource) => schema.toValues(editedValues?.(resource) ?? resource)
  return {
    id,
    open: ({ resource }) => values(resource),
    fingerprint: valuesFingerprint,
    submit: (current) => {
      const parsed = schema.update.safeParse(current)
      return parsed.success ? { body: parsed.data, fingerprint: valuesFingerprint(current) } : null
    },
    save: async ({ subject, submission }) => {
      const { data } = await update({
        path: subject.path,
        body: submission.body,
        throwOnError: true,
      })
      return data
    },
    acknowledge: (submission, resource) => ({ resource, fingerprint: submission.fingerprint }),
    fold: (current, receipt) =>
      valuesFingerprint(current) === receipt.fingerprint
        ? { content: values(receipt.resource), replaces: true }
        : { content: current, replaces: false },
    operations,
  }
}
