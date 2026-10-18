import { Loader, Stack, Title } from '@mantine/core'
import { schemaResolver, useForm } from '@mantine/form'
import {
  type QueryKey,
  type UseMutationOptions,
  type UseQueryOptions,
  useQuery,
} from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import { type ComponentType, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type * as z from 'zod'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as workspaceRoute } from '../routes/workspaces/$slug/route.tsx'
import type { ApiErrorLike } from '../utils/apiErrors.ts'
import type { FormValues, ResourceFormSchema, ResourceValues } from './resourceFormSchema.ts'

// The form a resource's form component receives. Validation comes from the schema (parse
// direction, errors only); the same schema yields the payload via transformValues once
// validation has passed. (@mantine/form's own form type is not importable, its .d.mts
// re-exports a missing file, so it is derived from the hook; the members typed by the
// validation rules are left out, they are the module's business.)
export type ResourceForm<TPayload> = Omit<
  ReturnType<typeof useForm<FormValues, TPayload>>,
  'validate' | 'validateField' | 'isValid'
>

function useResourceForm<TPayload>(
  initialValues: FormValues,
  schema: z.ZodType<TPayload, FormValues>,
) {
  return useForm<FormValues, TPayload>({
    initialValues,
    validate: schemaResolver(schema, { sync: true }),
    transformValues: (values) => schema.parse(values),
  })
}

// What a resource's form component receives. `form` validates through the resource's form
// schema and `onSubmit` is handed the normalized API payload; the component only renders
// fields and buttons.
export type ResourceFormProps<TPayload> = {
  form: ResourceForm<TPayload>
  isPending: boolean
  onSubmit: (payload: TPayload) => void
}

export type ResourceTexts = {
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
export type ResourceDescription<
  TPayload,
  TResource extends ResourceValues<TPayload>,
  TCreated extends { id: string },
  TError extends ApiErrorLike,
  TGetKey extends QueryKey,
  TIdParam extends string,
> = {
  // Form values <-> API payload, derived from the generated zod schema (resourceFormSchema).
  schema: ResourceFormSchema<TPayload>
  // The resource's field-rendering form.
  Form: ComponentType<ResourceFormProps<TPayload>>
  // The generated react-query pieces, passed as generated. The module calls them with
  // { path: { slug } } (list, create) or { path: { slug, id } } (get, update).
  getOptions: (options: {
    path: { slug: string; id: string }
  }) => UseQueryOptions<TResource, TError, TResource, TGetKey>
  createMutation: () => UseMutationOptions<
    TCreated,
    TError,
    { path: { slug: string }; body: TPayload }
  >
  updateMutation: () => UseMutationOptions<
    TResource,
    TError,
    { path: { slug: string; id: string }; body: TPayload }
  >
  listQueryKey: (options: { path: { slug: string } }) => QueryKey
  getQueryKey: (options: { path: { slug: string; id: string } }) => QueryKey
  // The edit route and the name of its param that holds the resource id, e.g. 'contactId'.
  editRoute: { useParams: () => Record<TIdParam, string> }
  idParam: TIdParam
  // All user-facing strings, resolved with the i18n `t` so keys stay statically extracted.
  texts: (t: TFunction) => ResourceTexts
  // Where to go after creating: the edit route, expressed with route references.
  editTarget: (
    created: TCreated,
    params: { slug: string },
  ) => { to: string; params: Record<string, string> }
}

export type ResourceEditPageProps<TResource> = {
  // Extra content rendered under the form, given the loaded resource (e.g. delivery actions).
  children?: (loaded: TResource) => ReactNode
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
    const { slug } = workspaceRoute.useParams()
    const texts = resource.texts(t)

    const form = useResourceForm(resource.schema.blank, resource.schema.create)

    const mutation = useResourceMutation({
      mutation: resource.createMutation(),
      invalidate: [resource.listQueryKey({ path: { slug } })],
      successMessage: texts.created,
      errorTitle: texts.saveErrorTitle,
      onDone: (created) => navigate(resource.editTarget(created, { slug })),
    })

    return (
      <Stack>
        <Title order={4}>{texts.createTitle}</Title>
        <resource.Form
          form={form}
          isPending={mutation.isPending}
          onSubmit={(body) => mutation.mutate({ path: { slug }, body })}
        />
      </Stack>
    )
  }

  // Mounted only once the resource is loaded, so the form is created with the hydrated values
  // (the schema's encode direction) and never renders blank. It is keyed by resource id by the
  // caller and not by the fetched data: a background refetch (invalidation, window focus) must
  // not remount the form and clobber unsaved edits. After a successful save the form is reset
  // to the saved resource instead, so it is clean and shows the server's normalized values.
  function EditForm({
    slug,
    id,
    loaded,
    children,
  }: ResourceEditPageProps<TResource> & { slug: string; id: string; loaded: TResource }) {
    const { t } = useTranslation()
    const texts = resource.texts(t)

    const form = useResourceForm(resource.schema.toValues(loaded), resource.schema.update)

    const mutation = useResourceMutation({
      mutation: resource.updateMutation(),
      invalidate: [
        resource.listQueryKey({ path: { slug } }),
        resource.getQueryKey({ path: { slug, id } }),
      ],
      successMessage: texts.updated,
      errorTitle: texts.saveErrorTitle,
      onDone: (saved) => {
        const values = resource.schema.toValues(saved)
        form.setValues(values)
        form.resetDirty(values)
      },
    })

    return (
      <Stack>
        <Title order={4}>{texts.editTitle}</Title>
        <resource.Form
          form={form}
          isPending={mutation.isPending}
          onSubmit={(body) => mutation.mutate({ path: { slug, id }, body })}
        />
        {children?.(loaded)}
      </Stack>
    )
  }

  function EditPage({ children }: ResourceEditPageProps<TResource>) {
    const { t } = useTranslation()
    const { slug } = workspaceRoute.useParams()
    const id = resource.editRoute.useParams()[resource.idParam]
    const texts = resource.texts(t)

    const query = useQuery(resource.getOptions({ path: { slug, id } }))

    if (query.isLoading) return <Loader />
    if (query.isError || !query.data) {
      return (
        <ApiErrorAlert
          error={query.error}
          title={texts.loadErrorTitle}
          fallback={texts.loadErrorTitle}
        />
      )
    }

    return (
      <EditForm key={id} slug={slug} id={id} loaded={query.data} {...(children && { children })} />
    )
  }

  return { CreatePage, EditPage }
}
