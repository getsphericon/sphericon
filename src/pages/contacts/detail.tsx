import {
  Avatar,
  EmptyState,
  Button,
  Badge,
  Group,
  SimpleGrid,
  Stack,
  Switch,
  Table,
  Text,
  Title,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { CountryFlag } from '../../components/CountryFlag.tsx'
import { EventRowDetails } from '../../components/EventRowDetails.tsx'
import { EventSummary } from '../../components/EventSummary.tsx'
import { RelativeTime } from '../../components/RelativeTime.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteContactsDeleteMutation,
  siteContactsGetOptions,
  siteEventsListOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteContactResource } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useContactExport } from '../../hooks/useContactExport.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { withPage } from '../../searchSchemas.ts'
import { contactFullName, contactName } from '../../utils/contactName.ts'
import { countryName } from '../../utils/countries.ts'
import { formatLongDate } from '../../utils/datetime.ts'
import { can } from '../../utils/permissions.ts'

const EVENTS_PAGE_SIZE = 10

interface Attribute {
  label: string
  value: string
  icon?: ReactNode
}

// A label beside its value, one row per attribute; an attribute the Contact has none of shows a
// dash.
function AttributeTable({ rows }: { rows: Attribute[] }) {
  return (
    <Table variant="vertical" layout="fixed" withTableBorder>
      <Table.Tbody>
        {rows.map(({ label, value, icon }) => (
          <Table.Tr key={label}>
            <Table.Th w={160}>{label}</Table.Th>
            <Table.Td>
              <Group gap="xs" wrap="nowrap">
                {icon}
                <Text size="sm">{value || '—'}</Text>
              </Group>
            </Table.Td>
          </Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
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
        <ApiErrorAlert error={eventsQuery.error} title={t(($) => $.activity.loadErrorTitle)} />
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
            render: (record) => <EventSummary workspaceId={workspaceId} event={record} />,
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

// The core attributes of the Contact in two tables side by side: who the Contact is, then where
// and when.
function ContactDetails({ contact }: { contact: SiteContactResource }) {
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  return (
    <SimpleGrid cols={{ base: 1, md: 2 }}>
      <AttributeTable
        rows={[
          { label: t(($) => $.table.email), value: contact.email ?? '' },
          { label: t(($) => $.table.subjectId), value: contact.subjectId ?? '' },
          { label: t(($) => $.table.phone), value: contact.phone ?? '' },
          { label: t(($) => $.table.firstName), value: contact.firstName ?? '' },
          { label: t(($) => $.table.lastName), value: contact.lastName ?? '' },
          {
            label: t(($) => $.table.birthday),
            value: contact.birthday ? formatLongDate(contact.birthday) : '',
          },
        ]}
      />
      <AttributeTable
        rows={[
          {
            label: t(($) => $.table.country),
            value: contact.country ? countryName(contact.country) : '',
            icon: contact.country ? <CountryFlag code={contact.country} /> : null,
          },
          { label: t(($) => $.table.city), value: contact.city ?? '' },
          { label: t(($) => $.table.timeZone), value: contact.timeZone ?? '' },
          { label: t(($) => $.table.locale), value: contact.locale ?? '' },
          { label: t(($) => $.activity.createdAt), value: formatDateTime(contact.createdAt) },
          { label: t(($) => $.contacts.updatedAt), value: formatDateTime(contact.updatedAt) },
        ]}
      />
    </SimpleGrid>
  )
}

export function ContactDetailPage() {
  const { t } = useTranslation()
  const { workspaceId, contactId } = contactsDetailRoute.useParams()
  const contactQuery = useQuery(siteContactsGetOptions({ path: { workspaceId, id: contactId } }))

  const contact = contactQuery.data

  if (contactQuery.isLoading) {
    return <PageSkeleton />
  }

  if (contactQuery.isError || !contact) {
    return (
      <ApiErrorAlert error={contactQuery.error} title={t(($) => $.alerts.contactLoadErrorTitle)} />
    )
  }

  const fullName = contactFullName(contact)
  const customFields = Object.entries(contact.customFields ?? {})

  return (
    <Stack>
      <Group justify="space-between" align="flex-start" wrap="wrap">
        <Group gap="md" wrap="nowrap">
          <Avatar
            size="lg"
            color="initials"
            name={contactName(contact)}
            src={contact.avatar ?? null}
            alt={contactName(contact)}
          />
          <Stack gap={4}>
            <Title order={4}>{contact.email ?? contact.subjectId ?? contact.phone ?? '—'}</Title>
            {fullName ? <Text c="dimmed">{fullName}</Text> : null}
          </Stack>
        </Group>
        <ContactActions workspaceId={workspaceId} contactId={contactId} />
      </Group>

      <Title order={5}>{t(($) => $.contacts.detailsTitle)}</Title>
      <ContactDetails contact={contact} />

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
          <AttributeTable
            rows={customFields.map(([key, value]) => ({
              label: key,
              value: formatCustomValue(value),
            }))}
          />
        </>
      ) : null}

      <ContactEvents workspaceId={workspaceId} contactId={contactId} />
    </Stack>
  )
}
