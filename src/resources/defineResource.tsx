import { Fieldset, Modal, Stack, Title } from '@mantine/core'
import { useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import type { QueryKey, UseMutationOptions, UseSuspenseQueryOptions } from '@tanstack/react-query'
import { Navigate, useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { LeaveDialog } from '../components/LeaveDialog.tsx'
import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { PageSkeleton } from '../components/Skeletons.tsx'
import { ChangedNotice } from '../editing/ChangedNotice.tsx'
import { defineEditorSession } from '../editing/editorSession.ts'
import { useEditorEffects } from '../editing/useEditorEffects.ts'
import type { EditorOutcomeTexts } from '../editing/useEditorEffects.ts'
import { useEditorSession } from '../editing/useEditorSession.ts'
import type { EditorOutcome } from '../editing/useEditorSession.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as workspaceRoute } from '../routes/workspaces/$workspaceId/route.tsx'
import type { ApiErrorLike } from '../utils/apiErrors.ts'
import { resourceEditorAdapter } from './resourceEditor.ts'
import type { ResourceOperation, ResourceUpdate } from './resourceEditor.ts'
import type { ResourceFormSchema, ResourceValues } from './resourceFormSchema.ts'
import { useSchemaForm } from './schemaForm.ts'
import type { SchemaForm } from './schemaForm.ts'

// The form a resource's form component receives. Validation comes from the schema (parse
// direction, errors only); the same schema yields the payload via transformValues once
// validation has passed.
type ResourceForm<TPayload, TValues extends object> = SchemaForm<TValues, TPayload>

// What a resource's form component receives. `form` validates through the resource's form
// schema and `onSubmit` is handed the normalized API payload; the component only renders
// fields and buttons. It never reshapes the payload: the edit page's session sends the schema's
// output of the values it captures, so whatever makes the request body belongs to the schema.
export interface ResourceFormProps<TPayload, TValues extends object> {
  form: ResourceForm<TPayload, TValues>
  isPending: boolean
  onSubmit: (payload: TPayload) => void
  // Set when the form is shown in a modal: Cancel closes it instead of leaving the page.
  onCancel?: () => void
}

interface ResourceTexts {
  createTitle: string
  editTitle: string
  created: string
  updated: string
  loadErrorTitle: string
  saveErrorTitle: string
}

// An operation of the edit page besides Save, with the toasts of its outcome.
export interface ResourceEditOperation<TResource, TValues> extends ResourceOperation<
  TResource,
  TValues
> {
  texts: (t: TFunction) => EditorOutcomeTexts
}

// What the content under the edit form receives of its editor session: whether the form holds
// unsaved edits, whether a write is pending and which operation it is, and the operations.
export interface ResourceEditor<TOperation extends string> {
  dirty: boolean
  busy: boolean
  operation: TOperation | null
  can: (operation: TOperation) => boolean
  // Starts the operation and resolves with its outcome (`busy` when it may not start now).
  run: (operation: TOperation, input?: unknown) => Promise<EditorOutcome>
}

// The whole interface of the resource form lifecycle module: describe a resource once and
// get a create page and an edit page that own load/error states, hydration, validation,
// normalization, writes, invalidation and post-create navigation. The edit page runs an editor
// session (docs/standards/ui.md, "Editor sessions"). The generic parameters are inferred from
// the generated pieces; callers never write them.
export interface ResourceDescription<
  TPayload,
  TValues extends object,
  TResource extends ResourceValues<TPayload>,
  TCreated extends { id: string },
  TError extends ApiErrorLike,
  TGetKey extends QueryKey,
  TIdParam extends string,
  TOperation extends string,
> {
  // Form values <-> API payload, derived from the generated zod schema (resourceFormSchema).
  schema: ResourceFormSchema<TPayload, TValues>
  // The resource's field-rendering form.
  Form: ComponentType<ResourceFormProps<TPayload, TValues>>
  // The generated pieces, passed as generated. The module calls them with
  // { path: { workspaceId } } (create) or { path: { workspaceId, id } } (get, update).
  getOptions: (options: {
    path: { workspaceId: string; id: string }
  }) => UseSuspenseQueryOptions<TResource, TError, TResource, TGetKey>
  createMutation: () => UseMutationOptions<
    TCreated,
    TError,
    { path: { workspaceId: string }; body: TPayload }
  >
  // The generated client call that saves an edit.
  update: ResourceUpdate<TPayload, TResource>
  // The edit page's operations besides Save, named by their event (PUBLISH, SEND, …); `{}` for a
  // resource that has none.
  operations: Record<TOperation, ResourceEditOperation<TResource, TValues>>
  // The edit route and the name of its param that holds the resource id, e.g. 'contactId'.
  editRoute: { useParams: () => Record<TIdParam, string> }
  idParam: TIdParam
  // All user-facing strings, resolved with the i18n `t` so keys stay statically extracted.
  texts: (t: TFunction) => ResourceTexts
  // Where to go after creating: the edit route, expressed with route references.
  editTarget: (
    created: TCreated,
    params: { workspaceId: string },
  ) => { to: string; params: Record<string, string> }
  // The edited values of a loaded resource whose fields are not at its top level (a Rule keeps
  // its definition in its draft or live version). Without it the resource is edited as it is.
  editedValues?: (loaded: TResource) => ResourceValues<TPayload>
  // The page the edit URL of a resource that takes no edit opens instead of the form (an archived
  // one, read-only until it is unarchived, ADR 0100); undefined for a resource that is edited.
  readOnlyTarget?: (
    loaded: TResource,
    params: { workspaceId: string; id: string },
  ) => { to: string; params: Record<string, string> } | undefined
}

export interface ResourceEditPageProps<TResource, TOperation extends string> {
  // Extra content rendered under the form (e.g. delivery actions), given the stored resource and
  // the editor session.
  children?: (loaded: TResource, editor: ResourceEditor<TOperation>) => ReactNode
}

export function defineResource<
  TPayload,
  TValues extends object,
  TResource extends ResourceValues<TPayload>,
  TCreated extends { id: string },
  TError extends ApiErrorLike,
  TGetKey extends QueryKey,
  TIdParam extends string,
  TOperation extends string,
>(
  resource: ResourceDescription<
    TPayload,
    TValues,
    TResource,
    TCreated,
    TError,
    TGetKey,
    TIdParam,
    TOperation
  >,
) {
  const { operations } = resource
  const adapter = resourceEditorAdapter({
    id: `${resource.idParam}Editor`,
    schema: resource.schema,
    editedValues: resource.editedValues,
    update: resource.update,
    operations,
  })
  const editorSession = defineEditorSession(adapter)

  function CreatePage() {
    const { t } = useTranslation()
    const navigate = useNavigate()
    const { workspaceId } = workspaceRoute.useParams()
    const texts = resource.texts(t)

    const form = useSchemaForm(resource.schema.create, resource.schema.blank)

    const mutation = useResourceMutation({
      mutation: resource.createMutation(),
      successMessage: texts.created,
      errorTitle: texts.saveErrorTitle,
      onDone: async (created) => navigate(resource.editTarget(created, { workspaceId })),
    })

    return (
      <Stack>
        <Title order={4}>{texts.createTitle}</Title>
        <resource.Form
          form={form}
          isPending={mutation.isPending}
          onSubmit={(body) => mutation.mutate({ path: { workspaceId }, body })}
        />
        {/* A save in flight is not blocked: it leaves for the created resource on success. */}
        <LeaveDialog
          dirty={form.isDirty() && !mutation.isPending}
          message={t(($) => $.leave.message)}
        />
      </Stack>
    )
  }

  // The create form in a modal over the list: a success closes it and refreshes the list, the
  // person stays where they were. The body mounts with the modal, so every opening starts blank.
  function CreateModalBody({ onClose }: { onClose: () => void }) {
    const { t } = useTranslation()
    const { workspaceId } = workspaceRoute.useParams()
    const texts = resource.texts(t)

    const form = useSchemaForm(resource.schema.create, resource.schema.blank)

    const mutation = useResourceMutation({
      mutation: resource.createMutation(),
      successMessage: texts.created,
      errorTitle: texts.saveErrorTitle,
      onDone: onClose,
    })

    return (
      <resource.Form
        form={form}
        isPending={mutation.isPending}
        onSubmit={(body) => mutation.mutate({ path: { workspaceId }, body })}
        onCancel={onClose}
      />
    )
  }

  function CreateModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
    const { t } = useTranslation()
    return (
      <Modal opened={opened} onClose={onClose} title={resource.texts(t).createTitle}>
        <CreateModalBody onClose={onClose} />
      </Modal>
    )
  }

  // Mounted only once the resource is loaded, so the form is created with the hydrated values
  // (the schema's encode direction) and never renders blank. It is keyed by resource id by the
  // caller and not by the fetched data: a refetch updates the session's metadata only, never the
  // form. Only the session replaces the form's values, on Revert or on a Save the form still
  // holds, and then remounts its fields, whose stateful editors read their initial value once.
  function EditForm({
    workspaceId,
    id,
    loaded,
    children,
  }: ResourceEditPageProps<TResource, TOperation> & {
    workspaceId: string
    id: string
    loaded: TResource
  }) {
    const { t } = useTranslation()
    const texts = resource.texts(t)

    const path = { workspaceId, id }
    // The values the session opens with, the same the form starts from.
    const form = useSchemaForm(resource.schema.update, adapter.open({ path, resource: loaded }))
    const effects = useEditorEffects<TResource, TOperation>(
      resource.getOptions({ path }).queryKey,
      (operation) =>
        operation === 'SAVE'
          ? { accepted: texts.updated, failedTitle: texts.saveErrorTitle }
          : operations[operation].texts(t),
    )

    const {
      snapshot: session,
      dirty,
      wouldLose,
      can,
      run,
      save,
      reload,
      overwrite,
    } = useEditorSession(adapter, editorSession, {
      subject: { path, resource: loaded },
      editor: {
        read: () => form.getValues(),
        // A changed field's error goes with its old value (clearInputErrorOnChange).
        write: (content) => {
          form.setValues(content)
        },
      },
      effects,
    })

    // The resource as stored now, read again for the changed notice's two actions.
    const queryClient = useQueryClient()
    const stored = async (then: (resource: TResource) => unknown) => {
      const { queryKey } = resource.getOptions({ path })
      await queryClient.refetchQueries({ queryKey, exact: true }, { throwOnError: true })
      const fresh = queryClient.getQueryData<TResource>(queryKey)
      if (fresh !== undefined) {
        await then(fresh)
      }
    }

    // Subscribed: the page re-renders on every edit.
    const current = form.values
    const editor: ResourceEditor<TOperation> = {
      dirty: dirty(current),
      busy: !session.matches('idle'),
      operation: session.context.operation,
      can,
      run,
    }

    return (
      <Stack>
        <Title order={4}>{texts.editTitle}</Title>
        {session.context.changed && (
          <ChangedNotice
            dirty={dirty(current)}
            busy={!session.matches('idle')}
            onLoad={() => {
              void stored(reload)
            }}
            onOverwrite={() => {
              void stored(overwrite)
            }}
          />
        )}
        {/* Fields stay enabled during Save; the other operations act on the stored resource. */}
        <Fieldset variant="unstyled" disabled={session.hasTag('readOnly')}>
          <resource.Form
            key={session.context.content.revision}
            form={form}
            isPending={session.matches('saving')}
            onSubmit={() => {
              void save((submission) => submission !== null)
            }}
          />
        </Fieldset>
        {children?.(session.context.resource, editor)}
        <LeaveDialog dirty={wouldLose(current)} message={t(($) => $.leave.message)} />
      </Stack>
    )
  }

  // The loaded resource; the edit page's boundary shows its load and a failed load.
  function LoadedEditForm({ children }: ResourceEditPageProps<TResource, TOperation>) {
    const { workspaceId } = workspaceRoute.useParams()
    const id = resource.editRoute.useParams()[resource.idParam]
    const { data } = useSuspenseQuery(resource.getOptions({ path: { workspaceId, id } }))

    const readOnly = resource.readOnlyTarget?.(data, { workspaceId, id })
    if (readOnly) {
      return <Navigate {...readOnly} replace />
    }

    return (
      <EditForm
        key={id}
        workspaceId={workspaceId}
        id={id}
        loaded={data}
        {...(children && { children })}
      />
    )
  }

  function EditPage({ children }: ResourceEditPageProps<TResource, TOperation>) {
    const { t } = useTranslation()
    return (
      <QueryBoundary fallback={<PageSkeleton />} errorTitle={resource.texts(t).loadErrorTitle}>
        <LoadedEditForm {...(children && { children })} />
      </QueryBoundary>
    )
  }

  return { CreatePage, CreateModal, EditPage }
}
