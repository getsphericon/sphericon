import { Modal, Stack, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { QueryKey, UseMutationOptions, UseSuspenseQueryOptions } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import type { ComponentType, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { LeaveDialog } from '../components/LeaveDialog.tsx'
import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { PageSkeleton } from '../components/Skeletons.tsx'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as workspaceRoute } from '../routes/workspaces/$workspaceId/route.tsx'
import type { ApiErrorLike } from '../utils/apiErrors.ts'
import type { FormValues, ResourceFormSchema, ResourceValues } from './resourceFormSchema.ts'
import { useSchemaForm } from './schemaForm.ts'
import type { SchemaForm } from './schemaForm.ts'

// The form a resource's form component receives. Validation comes from the schema (parse
// direction, errors only); the same schema yields the payload via transformValues once
// validation has passed.
export type ResourceForm<TPayload> = SchemaForm<FormValues, TPayload>

// What a resource's form component receives. `form` validates through the resource's form
// schema and `onSubmit` is handed the normalized API payload; the component only renders
// fields and buttons.
export interface ResourceFormProps<TPayload> {
  form: ResourceForm<TPayload>
  isPending: boolean
  onSubmit: (payload: TPayload) => void
  // Set when the form is shown in a modal: Cancel closes it instead of leaving the page.
  onCancel?: () => void
}

export interface ResourceTexts {
  createTitle: string
  editTitle: string
  created: string
  updated: string
  loadErrorTitle: string
  saveErrorTitle: string
}

// The whole interface of the resource form lifecycle module: describe a resource once and
// get a create page and an edit page that own load/error states, hydration, validation,
// normalization, mutation, invalidation and post-create navigation. The generic parameters
// are inferred from the generated pieces; callers never write them.
export interface ResourceDescription<
  TPayload,
  TResource extends ResourceValues<TPayload>,
  TCreated extends { id: string },
  TError extends ApiErrorLike,
  TGetKey extends QueryKey,
  TIdParam extends string,
> {
  // Form values <-> API payload, derived from the generated zod schema (resourceFormSchema).
  schema: ResourceFormSchema<TPayload>
  // The resource's field-rendering form.
  Form: ComponentType<ResourceFormProps<TPayload>>
  // The generated react-query pieces, passed as generated. The module calls them with
  // { path: { workspaceId } } (list, create) or { path: { workspaceId, id } } (get, update).
  getOptions: (options: {
    path: { workspaceId: string; id: string }
  }) => UseSuspenseQueryOptions<TResource, TError, TResource, TGetKey>
  createMutation: () => UseMutationOptions<
    TCreated,
    TError,
    { path: { workspaceId: string }; body: TPayload }
  >
  updateMutation: () => UseMutationOptions<
    TResource,
    TError,
    { path: { workspaceId: string; id: string }; body: TPayload }
  >
  listQueryKey: (options: { path: { workspaceId: string } }) => QueryKey
  getQueryKey: (options: { path: { workspaceId: string; id: string } }) => QueryKey
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
  // Which revision of the resource the form edits, when something besides the form's own save
  // can replace what it edits (a Rule's draft dropped by Revert): a new revision reloads the form.
  revision?: (loaded: TResource) => string
}

export interface ResourceEditPageProps<TResource> {
  // Extra content rendered under the form, given the loaded resource (e.g. delivery actions) and
  // whether the form holds unsaved changes.
  children?: (loaded: TResource, form: { dirty: boolean }) => ReactNode
}

export function defineResource<
  TPayload,
  TResource extends ResourceValues<TPayload>,
  TCreated extends { id: string },
  TError extends ApiErrorLike,
  TGetKey extends QueryKey,
  TIdParam extends string,
>(resource: ResourceDescription<TPayload, TResource, TCreated, TError, TGetKey, TIdParam>) {
  function CreatePage() {
    const { t } = useTranslation()
    const navigate = useNavigate()
    const { workspaceId } = workspaceRoute.useParams()
    const texts = resource.texts(t)

    const form = useSchemaForm(resource.schema.create, resource.schema.blank)

    const mutation = useResourceMutation({
      mutation: resource.createMutation(),
      invalidate: [resource.listQueryKey({ path: { workspaceId } })],
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
      invalidate: [resource.listQueryKey({ path: { workspaceId } })],
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
  // caller and not by the fetched data: a background refetch (invalidation, window focus) must
  // not remount the form and clobber unsaved edits. After a successful save the form is reset
  // to the saved resource instead, so it is clean and shows the server's normalized values. A
  // resource with revisions is keyed by its revision too: when another action replaces what the
  // form edits, the form reloads.
  function EditForm({
    workspaceId,
    id,
    loaded,
    children,
  }: ResourceEditPageProps<TResource> & { workspaceId: string; id: string; loaded: TResource }) {
    const { t } = useTranslation()
    const texts = resource.texts(t)

    const values = (from: TResource) =>
      resource.schema.toValues(resource.editedValues?.(from) ?? from)
    const form = useSchemaForm(resource.schema.update, values(loaded))

    const mutation = useResourceMutation({
      mutation: resource.updateMutation(),
      invalidate: [
        resource.listQueryKey({ path: { workspaceId } }),
        resource.getQueryKey({ path: { workspaceId, id } }),
      ],
      successMessage: texts.updated,
      errorTitle: texts.saveErrorTitle,
      onDone: (saved) => {
        const next = values(saved)
        form.setValues(next)
        form.resetDirty(next)
      },
    })

    return (
      <Stack>
        <Title order={4}>{texts.editTitle}</Title>
        <resource.Form
          form={form}
          isPending={mutation.isPending}
          onSubmit={(body) => mutation.mutate({ path: { workspaceId, id }, body })}
        />
        {children?.(loaded, { dirty: form.isDirty() })}
        <LeaveDialog
          dirty={form.isDirty() && !mutation.isPending}
          message={t(($) => $.leave.message)}
        />
      </Stack>
    )
  }

  // The loaded resource; the edit page's boundary shows its load and a failed load.
  function LoadedEditForm({ children }: ResourceEditPageProps<TResource>) {
    const { workspaceId } = workspaceRoute.useParams()
    const id = resource.editRoute.useParams()[resource.idParam]
    const { data } = useSuspenseQuery(resource.getOptions({ path: { workspaceId, id } }))

    return (
      <EditForm
        key={`${id}:${resource.revision?.(data) ?? ''}`}
        workspaceId={workspaceId}
        id={id}
        loaded={data}
        {...(children && { children })}
      />
    )
  }

  function EditPage({ children }: ResourceEditPageProps<TResource>) {
    const { t } = useTranslation()
    return (
      <QueryBoundary fallback={<PageSkeleton />} errorTitle={resource.texts(t).loadErrorTitle}>
        <LoadedEditForm {...(children && { children })} />
      </QueryBoundary>
    )
  }

  return { CreatePage, CreateModal, EditPage }
}
