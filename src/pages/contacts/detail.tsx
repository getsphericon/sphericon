import {
  EmptyState,
  Button,
  Card,
  Badge,
  Group,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { EventActionName } from '../../components/EventActionName.tsx'
import { EventRowDetails } from '../../components/EventRowDetails.tsx'
import { RelativeTime } from '../../components/RelativeTime.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteContactsDeleteMutation,
  siteContactsGetOptions,
  siteEventsListOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useContactExport } from '../../hooks/useContactExport.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { withPage } from '../../searchSchemas.ts'
import { can } from '../../utils/permissions.ts'

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

function ContactActions({ workspaceId, contactId }: { workspaceId: string; contactId: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const confirm = useConfirmation()
  const workspaces = useQuery(siteWorkspacesListOptions())
  const workspace = workspaces.data?.find((w) => w.id === workspaceId)
  // Erasure is irreversible, so it is offered only with contacts:erase, and exporting only
  // with contacts:export (the API refuses everyone else with 403).
  const canErase = can(workspace, 'contacts:erase')
  const canExport = can(workspace, 'contacts:export')

  const eraseMutation = useResourceMutation({
    mutation: siteContactsDeleteMutation(),
    successMessage: t(($) => $.notifications.contactErased),
    errorTitle: t(($) => $.alerts.eraseErrorTitle),
    forbiddenMessage: t(($) => $.contacts.eraseForbidden),
    onDone: async () => navigate({ to: contactsRoute.to, params: { workspaceId } }),
  })

  const exportMutation = useContactExport(workspaceId)

  return (
    <Group gap="xs">
      <ButtonLink variant="default" to={contactsEditRoute.to} params={{ workspaceId, contactId }}>
        {t(($) => $.actions.edit)}
      </ButtonLink>
      {canExport ? (
        <Button
          variant="default"
          loading={exportMutation.isPending}
          onClick={() => exportMutation.mutate({ id: contactId })}
        >
          {t(($) => $.contacts.export)}
        </Button>
      ) : null}
      {canErase ? (
        <Button
          color="red"
          variant="light"
          onClick={() =>
            confirm({
              title: t(($) => $.contacts.eraseConfirmTitle),
              description: t(($) => $.contacts.eraseConfirmDescription),
              confirmLabel: t(($) => $.contacts.erase),
              onConfirm: () => eraseMutation.mutate({ path: { workspaceId, id: contactId } }),
            })
          }
        >
          {t(($) => $.contacts.erase)}
        </Button>
      ) : null}
      <ButtonLink variant="subtle" to={contactsRoute.to} params={{ workspaceId }}>
        {t(($) => $.actions.back)}
      </ButtonLink>
    </Group>
  )
}

function ContactEvents({ workspaceId, contactId }: { workspaceId: string; contactId: string }) {
  const { t } = useTranslation()
  const { page = 1, machine: showMachine = false } = contactsDetailRoute.useSearch()
  const navigate = contactsDetailRoute.useNavigate()
  const setPage = (next: number) =>
    void navigate({
      search: withPage(next),
      viewTransition: false,
    })

  // Activity is keyed by the stable contact_id (ADR 0002): this also surfaces
  // anonymous events stitched onto the contact at Identify, which an email filter
  // would miss.
  const eventsQuery = useQuery(
    siteEventsListOptions({
      path: { workspaceId },
      query: { page, pageSize: EVENTS_PAGE_SIZE, contactId, includeMachine: showMachine },
    }),
  )

  // A machine Event (an open or click made by software, ADR 0053) is left out by the API unless
  // `includeMachine` asks for it.
  const eventRecords = eventsQuery.data?.items ?? []
  const eventsTotal = eventsQuery.data?.totalItems ?? 0

  return (
    <>
      <Group justify="space-between">
        <Title order={5}>{t(($) => $.contacts.eventsTitle)}</Title>
        <Switch
          label={t(($) => $.contacts.showMachineEvents)}
          checked={showMachine}
          onChange={(e) =>
            // Another set of events: back to its first page.
            void navigate({
              search: { machine: e.currentTarget.checked || undefined },
              replace: true,
              viewTransition: false,
            })
          }
        />
      </Group>

      {eventsQuery.isError ? (
        <ApiErrorAlert
          error={eventsQuery.error}
          title={t(($) => $.activity.loadErrorTitle)}
          fallback={t(($) => $.activity.loadErrorTitle)}
        />
      ) : null}

      <DataTable
        minHeight={eventRecords.length === 0 ? 200 : 0}
        withTableBorder
        highlightOnHover
        records={eventRecords}
        idAccessor="id"
        columns={[
          {
            accessor: 'createdAt',
            title: t(($) => $.activity.time),
            render: (record) => <RelativeTime value={record.createdAt} />,
          },
          {
            accessor: 'action',
            title: t(($) => $.activity.action),
            render: (record) => <EventActionName action={record.action} />,
          },
        ]}
        rowExpansion={{ content: EventRowDetails }}
        totalRecords={eventsTotal}
        recordsPerPage={EVENTS_PAGE_SIZE}
        page={page}
        onPageChange={setPage}
        fetching={eventsQuery.isFetching}
        emptyState={<EmptyState title={t(($) => $.activity.noRecords)} />}
      />
    </>
  )
}

export function ContactDetailPage() {
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  const { workspaceId, contactId } = contactsDetailRoute.useParams()
  const contactQuery = useQuery(siteContactsGetOptions({ path: { workspaceId, id: contactId } }))

  const contact = contactQuery.data

  if (contactQuery.isLoading) {
    return <PageSkeleton />
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

  return (
    <Stack>
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Stack gap={4}>
          <Title order={4}>{contact.email ?? contact.subjectId ?? contact.phone ?? '—'}</Title>
          {fullName ? <Text c="dimmed">{fullName}</Text> : null}
        </Stack>
        <ContactActions workspaceId={workspaceId} contactId={contactId} />
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

      {contact.tags.length > 0 ? (
        <>
          <Title order={5}>{t(($) => $.contacts.tagsTitle)}</Title>
          <Group gap="xs">
            {contact.tags.map((tag) => (
              <Badge key={tag} variant="light">
                {tag}
              </Badge>
            ))}
          </Group>
        </>
      ) : null}

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

      <ContactEvents workspaceId={workspaceId} contactId={contactId} />
    </Stack>
  )
}
