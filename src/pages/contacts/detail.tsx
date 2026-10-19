import {
  EmptyState,
  Button,
  Card,
  Group,
  Loader,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { parse } from 'content-disposition-header'
import { saveAs } from 'file-saver'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { EventRowDetails } from '../../components/EventRowDetails.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteContactsDeleteMutation,
  siteContactsGetOptions,
  siteContactsListQueryKey,
  siteEventsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteContactsExport } from '../../generated/site/sdk.gen.ts'
import { canManage, useCurrentRole } from '../../hooks/useCurrentRole.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$slug/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$slug/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$slug/contacts/index.tsx'
import { getApiErrorMessage } from '../../utils/apiErrors.ts'
import { formatDateTime } from '../../utils/datetime.ts'

// The filename the server suggested in Content-Disposition (RFC 6266, `filename*` included).
function exportFilename(response: Response): string | undefined {
  const header = response.headers.get('Content-Disposition')
  return header ? parse(header).parameters.filename : undefined
}

const EVENTS_PAGE_SIZE = 10

function Field({ label, value }: { label: string; value: string }) {
  return (
    <Card withBorder padding="md">
      <Text size="xs" c="dimmed" tt="uppercase">
        {label}
      </Text>
      <Text fw={500}>{value || '—'}</Text>
    </Card>
  )
}

// Custom field values are typed (string/number/bool/…); render scalars directly and
// anything richer as compact JSON.
function formatCustomValue(value: unknown): string {
  if (value === null || value === undefined) {
    return ''
  }
  if (typeof value === 'string') {
    return value
  }
  return JSON.stringify(value)
}

export function ContactDetailPage() {
  const { t } = useTranslation()
  const { slug, contactId } = contactsDetailRoute.useParams()
  const navigate = useNavigate()
  const confirm = useDeleteConfirmation()
  const role = useCurrentRole(slug)
  // Erasure is irreversible, so it is offered to owners and admins only (the API
  // refuses everyone else with 403).
  const canErase = canManage(role)
  const [page, setPage] = useState(1)

  // The component instance is reused across contactId changes, so reset the
  // events page to avoid requesting an out-of-range page for the next contact
  // (derived during render, the documented alternative to setState in an effect).
  const [pageContactId, setPageContactId] = useState(contactId)
  if (pageContactId !== contactId) {
    setPageContactId(contactId)
    setPage(1)
  }

  const contactQuery = useQuery(siteContactsGetOptions({ path: { slug, id: contactId } }))

  const contact = contactQuery.data

  // Activity is keyed by the stable contact_id (ADR 0002): this also surfaces
  // anonymous events stitched onto the contact at Identify, which an email filter
  // would miss.
  const eventsQuery = useQuery(
    siteEventsListOptions({
      path: { slug },
      query: { page, pageSize: EVENTS_PAGE_SIZE, contactId },
    }),
  )

  const eraseMutation = useResourceMutation({
    mutation: siteContactsDeleteMutation(),
    invalidate: [siteContactsListQueryKey({ path: { slug } })],
    successMessage: t(($) => $.notifications.contactErased),
    errorTitle: t(($) => $.alerts.eraseErrorTitle),
    forbiddenMessage: t(($) => $.contacts.eraseForbidden),
    onDone: () => navigate({ to: contactsRoute.to, params: { slug } }),
  })

  const exportMutation = useMutation({
    // The generated mutation helper drops the response; the filename is the server's
    // (Content-Disposition), so call the generated SDK function and keep both. The
    // server streams the document as a download; it is read as the contract's typed
    // ContactExportDocument and written out as the saved file.
    mutationFn: () =>
      siteContactsExport({
        path: { slug },
        query: { id: contactId },
        parseAs: 'json',
        throwOnError: true,
      }),
    onSuccess: ({ data, response }) =>
      saveAs(
        new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }),
        exportFilename(response),
      ),
    onError: (error) =>
      notifications.show({
        color: 'red',
        title: t(($) => $.alerts.exportErrorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.notifications.errorMessage),
        ),
      }),
  })

  if (contactQuery.isLoading) {
    return <Loader />
  }

  if (contactQuery.isError || !contact) {
    return (
      <ApiErrorAlert
        error={contactQuery.error}
        title={t(($) => $.alerts.contactLoadErrorTitle)}
        fallback={t(($) => $.alerts.contactLoadErrorTitle)}
      />
    )
  }

  const fullName = [contact.firstName, contact.lastName].filter(Boolean).join(' ')
  const customFields = Object.entries(contact.customFields ?? {})
  const eventRecords = eventsQuery.data?.items ?? []
  const eventsTotal = eventsQuery.data?.totalItems ?? 0

  return (
    <Stack>
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Stack gap={4}>
          <Title order={4}>{contact.email ?? contact.subjectId ?? contact.phone ?? '—'}</Title>
          {fullName ? <Text c="dimmed">{fullName}</Text> : null}
        </Stack>
        <Group gap="xs">
          <ButtonLink variant="default" to={contactsEditRoute.to} params={{ slug, contactId }}>
            {t(($) => $.actions.edit)}
          </ButtonLink>
          <Button
            variant="default"
            loading={exportMutation.isPending}
            onClick={() => exportMutation.mutate()}
          >
            {t(($) => $.contacts.export)}
          </Button>
          {canErase ? (
            <Button
              color="red"
              variant="light"
              onClick={() =>
                confirm({
                  title: t(($) => $.contacts.eraseConfirmTitle),
                  description: t(($) => $.contacts.eraseConfirmDescription),
                  confirmLabel: t(($) => $.contacts.erase),
                  onConfirm: () => eraseMutation.mutate({ path: { slug, id: contactId } }),
                })
              }
            >
              {t(($) => $.contacts.erase)}
            </Button>
          ) : null}
          <ButtonLink variant="subtle" to={contactsRoute.to} params={{ slug }}>
            {t(($) => $.actions.back)}
          </ButtonLink>
        </Group>
      </Group>

      <Title order={5}>{t(($) => $.contacts.detailsTitle)}</Title>
      <SimpleGrid cols={{ base: 1, sm: 2, md: 3 }}>
        <Field label={t(($) => $.table.email)} value={contact.email ?? ''} />
        <Field label={t(($) => $.table.subjectId)} value={contact.subjectId ?? ''} />
        <Field label={t(($) => $.table.phone)} value={contact.phone ?? ''} />
        <Field label={t(($) => $.table.firstName)} value={contact.firstName ?? ''} />
        <Field label={t(($) => $.table.lastName)} value={contact.lastName ?? ''} />
        <Field label={t(($) => $.table.timeZone)} value={contact.timeZone ?? ''} />
        <Field label={t(($) => $.activity.createdAt)} value={formatDateTime(contact.createdAt)} />
        <Field label={t(($) => $.contacts.updatedAt)} value={formatDateTime(contact.updatedAt)} />
      </SimpleGrid>

      {customFields.length > 0 ? (
        <>
          <Title order={5}>{t(($) => $.contacts.customFields)}</Title>
          <SimpleGrid cols={{ base: 1, sm: 2, md: 3 }}>
            {customFields.map(([key, value]) => (
              <Field key={key} label={key} value={formatCustomValue(value)} />
            ))}
          </SimpleGrid>
        </>
      ) : null}

      <Title order={5}>{t(($) => $.contacts.eventsTitle)}</Title>

      {eventsQuery.isError ? (
        <ApiErrorAlert
          error={eventsQuery.error}
          title={t(($) => $.activity.loadErrorTitle)}
          fallback={t(($) => $.activity.loadErrorTitle)}
        />
      ) : null}

      <DataTable
        withTableBorder
        records={eventRecords}
        idAccessor="id"
        columns={[
          {
            accessor: 'createdAt',
            title: t(($) => $.activity.time),
            render: (record) => formatDateTime(record.createdAt),
          },
          { accessor: 'action', title: t(($) => $.activity.action) },
        ]}
        rowExpansion={{ content: EventRowDetails }}
        totalRecords={eventsTotal}
        recordsPerPage={EVENTS_PAGE_SIZE}
        page={page}
        onPageChange={setPage}
        fetching={eventsQuery.isFetching}
        emptyState={<EmptyState title={t(($) => $.activity.noRecords)} />}
      />
    </Stack>
  )
}
