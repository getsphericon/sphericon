import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Code,
  Group,
  Loader,
  NumberInput,
  Select,
  SimpleGrid,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { notifications } from '@mantine/notifications'
import { IconChevronDown, IconChevronRight, IconDownload } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { BillingBadge } from '../../components/BillingBadge.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAuditExportOptions,
  siteAuditGetRetentionOptions,
  siteAuditGetRetentionQueryKey,
  siteAuditListOptions,
  siteAuditSetRetentionMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAuditEntryResource } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as settingsRoute } from '../../routes/workspaces/$slug/settings.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import { formatDateTime } from '../../utils/datetime.ts'
import { auditQuery } from './auditFilter.ts'
import type { AuditFilter } from './auditFilter.ts'

const PAGE_SIZE = 25
// The API answers 402 without an Enterprise license and 403 to a plain member; in both
// cases the section is simply not offered.
const HIDDEN_STATUSES = [402, 403]

function isHidden(error: unknown) {
  return hasErrorStatus(error, HIDDEN_STATUSES)
}

function ActorLabel({ entry }: { entry: SiteAuditEntryResource }) {
  const { t } = useTranslation()
  const { kind, name, id } = entry.actor
  if (kind === 'system') {
    return t(($) => $.settings.auditLog.actorSystem)
  }
  if (kind === 'api_token') {
    return name ?? t(($) => $.settings.auditLog.actorToken, { id: id ?? '' })
  }
  return name ?? id ?? ''
}

function TargetLabel({ entry }: { entry: SiteAuditEntryResource }) {
  const { type, name, id } = entry.target
  return (
    <>
      {type}
      {(name ?? id) ? ` ${name ?? `#${id}`}` : ''}
    </>
  )
}

// DiffValue shows one side of a changed field.
function DiffValue({ value }: { value: unknown }) {
  const { t } = useTranslation()
  if (value === null || value === undefined || value === '') {
    return (
      <Text span c="dimmed" size="sm">
        {t(($) => $.settings.auditLog.emptyValue)}
      </Text>
    )
  }
  return <Code>{typeof value === 'string' ? value : JSON.stringify(value)}</Code>
}

function isChange(value: unknown): value is { from?: unknown; to?: unknown } {
  return typeof value === 'object' && value !== null && ('from' in value || 'to' in value)
}

// DiffView lists the changed fields. A field the schema marks sensitive is the bare string
// "changed": the log never holds its value, so only a marker is shown.
function DiffView({ diff }: { diff: Record<string, unknown> }) {
  const { t } = useTranslation()
  return (
    <Stack gap={4}>
      {Object.entries(diff).map(([field, value]) => (
        <Group key={field} gap="xs">
          <Text fw={600} size="sm">
            {field}
          </Text>
          {isChange(value) ? (
            <>
              <DiffValue value={value.from} />
              <Text span c="dimmed" size="sm">
                {'→'}
              </Text>
              <DiffValue value={value.to} />
            </>
          ) : (
            <Badge variant="light" color="yellow">
              {t(($) => $.settings.auditLog.changedMarker)}
            </Badge>
          )}
        </Group>
      ))}
    </Stack>
  )
}

// EntryRows is one entry; when it carries a diff, a chevron expands it beneath the row so
// the list stays scannable.
function EntryRows({ entry }: { entry: SiteAuditEntryResource }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const diff = entry.diff && Object.keys(entry.diff).length > 0 ? entry.diff : null

  return (
    <Fragment>
      <Table.Tr>
        <Table.Td>{formatDateTime(entry.occurredAt)}</Table.Td>
        <Table.Td>
          <ActorLabel entry={entry} />
        </Table.Td>
        <Table.Td>
          <Group gap="xs" wrap="nowrap">
            {entry.action}
            {entry.billing ? <BillingBadge /> : null}
          </Group>
        </Table.Td>
        <Table.Td>
          <TargetLabel entry={entry} />
        </Table.Td>
        <Table.Td>
          {diff ? (
            <ActionIcon
              variant="subtle"
              color="gray"
              aria-expanded={open}
              aria-label={
                open ? t(($) => $.settings.auditLog.collapse) : t(($) => $.settings.auditLog.expand)
              }
              onClick={() => setOpen((v) => !v)}
            >
              {open ? <IconChevronDown size={16} /> : <IconChevronRight size={16} />}
            </ActionIcon>
          ) : null}
        </Table.Td>
      </Table.Tr>
      {diff && open ? (
        <Table.Tr>
          <Table.Td colSpan={5}>
            <DiffView diff={diff} />
          </Table.Td>
        </Table.Tr>
      ) : null}
    </Fragment>
  )
}

// AuditPage renders one page of the log and, while more exist, a button that mounts the
// next page beneath it, so the list grows without holding every page in one state.
function AuditPage({
  slug,
  filter,
  cursor,
  first,
}: {
  slug: string
  filter: AuditFilter
  cursor?: string
  first: boolean
}) {
  const { t } = useTranslation()
  const [showNext, setShowNext] = useState(false)
  const query = useQuery(
    siteAuditListOptions({
      path: { slug },
      query: { ...auditQuery(filter), limit: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
    }),
  )

  if (query.isLoading) {
    return <Loader size="sm" />
  }
  if (isHidden(query.error)) {
    return null
  }
  if (query.isError) {
    return (
      <Alert color="red" title={t(($) => $.settings.auditLog.loadError)}>
        {t(($) => $.settings.auditLog.loadError)}
      </Alert>
    )
  }

  const items = query.data?.items ?? []
  const next = query.data?.nextCursor ?? undefined
  if (first && items.length === 0) {
    return (
      <Text c="dimmed" size="sm">
        {Object.keys(auditQuery(filter)).length > 0
          ? t(($) => $.settings.auditLog.noMatches)
          : t(($) => $.settings.auditLog.empty)}
      </Text>
    )
  }

  return (
    <>
      <Table.ScrollContainer minWidth={640}>
        <Table withTableBorder>
          {first ? (
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t(($) => $.settings.auditLog.time)}</Table.Th>
                <Table.Th>{t(($) => $.settings.auditLog.actor)}</Table.Th>
                <Table.Th>{t(($) => $.settings.auditLog.action)}</Table.Th>
                <Table.Th>{t(($) => $.settings.auditLog.target)}</Table.Th>
                <Table.Th w={40} />
              </Table.Tr>
            </Table.Thead>
          ) : null}
          <Table.Tbody>
            {items.map((entry) => (
              <EntryRows key={entry.id} entry={entry} />
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
      {next && !showNext ? (
        <Button variant="light" onClick={() => setShowNext(true)}>
          {t(($) => $.settings.auditLog.loadMore)}
        </Button>
      ) : null}
      {next && showNext ? (
        <AuditPage slug={slug} filter={filter} cursor={next} first={false} />
      ) : null}
    </>
  )
}

// saveCsv hands the exported text to the browser as a file download.
function saveCsv(csv: string, filename: string) {
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }))
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

// ExportButton exports the log narrowed by the filter currently applied, so the file
// matches what the page shows.
function ExportButton({ slug, filter }: { slug: string; filter: AuditFilter }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const exportCsv = useMutation({
    mutationFn: () =>
      queryClient.query({
        ...siteAuditExportOptions({ path: { slug }, query: auditQuery(filter) }),
        staleTime: 0,
        gcTime: 0,
      }),
    onSuccess: (csv) => saveCsv(csv, `audit-log-${slug}.csv`),
    onError: () =>
      notifications.show({
        color: 'red',
        message: t(($) => $.settings.auditLog.exportError),
      }),
  })

  return (
    <Button
      variant="default"
      size="xs"
      leftSection={<IconDownload size={14} />}
      loading={exportCsv.isPending}
      onClick={() => exportCsv.mutate()}
    >
      {t(($) => $.settings.auditLog.exportCsv)}
    </Button>
  )
}

// RetentionForm sets how many days the log is kept; an empty value keeps it forever. The
// change is recorded as an Audit entry by the API.
function RetentionForm({ slug, days }: { slug: string; days: number | null }) {
  const { t } = useTranslation()
  const [value, setValue] = useState<string | number>(days ?? '')
  const mutation = useResourceMutation({
    mutation: siteAuditSetRetentionMutation(),
    invalidate: [siteAuditGetRetentionQueryKey({ path: { slug } })],
    successMessage: t(($) => $.settings.auditLog.retentionSaved),
    errorTitle: t(($) => $.settings.auditLog.retentionError),
  })

  return (
    <Group align="flex-end" gap="sm">
      <NumberInput
        label={t(($) => $.settings.auditLog.retentionLabel)}
        description={t(($) => $.settings.auditLog.retentionDescription)}
        placeholder={t(($) => $.settings.auditLog.retentionPlaceholder)}
        min={1}
        max={3650}
        allowDecimal={false}
        value={value}
        onChange={setValue}
      />
      <Button
        size="xs"
        loading={mutation.isPending}
        onClick={() =>
          mutation.mutate({
            path: { slug },
            body: { retentionDays: typeof value === 'number' ? value : null },
          })
        }
      >
        {t(($) => $.settings.auditLog.retentionSave)}
      </Button>
    </Group>
  )
}

// RetentionControl is the Enterprise advanced-retention window for the log. It renders
// nothing where the setting is not offered (a plain member, or no license).
function RetentionControl({ slug }: { slug: string }) {
  const query = useQuery(siteAuditGetRetentionOptions({ path: { slug } }))
  if (!query.isSuccess) {
    return null
  }
  return (
    <RetentionForm
      key={String(query.data.retentionDays)}
      slug={slug}
      days={query.data.retentionDays}
    />
  )
}

const TEXT_FIELDS = ['actorId', 'action', 'targetType', 'targetId', 'ip', 'requestId'] as const

// FilterForm edits the filter and hands it up on apply, so the route's search stays the
// single owner of it. It remounts when the filter changes from outside, e.g. by following
// a Change history link.
function FilterForm({
  filter,
  onChange,
}: {
  filter: AuditFilter
  onChange: (filter: AuditFilter) => void
}) {
  const { t } = useTranslation()
  const form = useForm<{
    from: string
    to: string
    actorKind: string | null
    actorId: string
    action: string
    targetType: string
    targetId: string
    ip: string
    requestId: string
  }>({
    initialValues: {
      from: filter.from ?? '',
      to: filter.to ?? '',
      actorKind: filter.actorKind ?? null,
      actorId: filter.actorId ?? '',
      action: filter.action ?? '',
      targetType: filter.targetType ?? '',
      targetId: filter.targetId ?? '',
      ip: filter.ip ?? '',
      requestId: filter.requestId ?? '',
    },
  })

  const labels = {
    actorId: t(($) => $.settings.auditLog.actorId),
    action: t(($) => $.settings.auditLog.action),
    targetType: t(($) => $.settings.auditLog.targetType),
    targetId: t(($) => $.settings.auditLog.targetId),
    ip: t(($) => $.settings.auditLog.ip),
    requestId: t(($) => $.settings.auditLog.requestId),
  }

  return (
    <form
      onSubmit={form.onSubmit((values) => {
        const next: AuditFilter = {}
        for (const field of TEXT_FIELDS) {
          const value = values[field].trim()
          if (value) {
            next[field] = value
          }
        }
        if (values.from) {
          next.from = values.from
        }
        if (values.to) {
          next.to = values.to
        }
        const kind = auditFilterKind(values.actorKind)
        if (kind) {
          next.actorKind = kind
        }
        onChange(next)
      })}
    >
      <Stack gap="sm">
        <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }}>
          <TextInput
            type="date"
            label={t(($) => $.settings.auditLog.from)}
            {...form.getInputProps('from')}
          />
          <TextInput
            type="date"
            label={t(($) => $.settings.auditLog.to)}
            {...form.getInputProps('to')}
          />
          <Select
            clearable
            label={t(($) => $.settings.auditLog.actorKind)}
            data={[
              { value: 'user', label: t(($) => $.settings.auditLog.kindUser) },
              { value: 'api_token', label: t(($) => $.settings.auditLog.kindApiToken) },
              { value: 'staff', label: t(($) => $.settings.auditLog.kindStaff) },
              { value: 'system', label: t(($) => $.settings.auditLog.kindSystem) },
            ]}
            {...form.getInputProps('actorKind')}
          />
          <TextInput label={labels.actorId} {...form.getInputProps('actorId')} />
          <TextInput
            label={labels.action}
            placeholder={t(($) => $.settings.auditLog.actionPlaceholder)}
            {...form.getInputProps('action')}
          />
          <TextInput label={labels.targetType} {...form.getInputProps('targetType')} />
          <TextInput label={labels.targetId} {...form.getInputProps('targetId')} />
          <TextInput label={labels.ip} {...form.getInputProps('ip')} />
          <TextInput label={labels.requestId} {...form.getInputProps('requestId')} />
        </SimpleGrid>
        <Group gap="xs">
          <Button type="submit" size="xs">
            {t(($) => $.settings.auditLog.apply)}
          </Button>
          <Button type="button" size="xs" variant="default" onClick={() => onChange({})}>
            {t(($) => $.settings.auditLog.reset)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

const ACTOR_KINDS = ['user', 'api_token', 'staff', 'system'] as const

function auditFilterKind(value: string | null) {
  return ACTOR_KINDS.find((kind) => kind === value)
}

// ChangeHistoryLink opens the Audit log pre-filtered to one object (an Integration or a
// Webhook endpoint). It renders nothing where the log is not offered (a plain member, or
// no Enterprise license), so it never points at a section that is not there.
export function ChangeHistoryLink({
  slug,
  targetType,
  targetId,
}: {
  slug: string
  targetType: string
  targetId: string
}) {
  const { t } = useTranslation()
  const probe = useQuery(siteAuditListOptions({ path: { slug }, query: { limit: 1 } }))
  if (!probe.isSuccess) {
    return null
  }

  return (
    <ButtonLink
      size="compact-sm"
      variant="subtle"
      to={settingsRoute.to}
      params={{ slug }}
      search={{ targetType, targetId }}
    >
      {t(($) => $.settings.auditLog.changeHistory)}
    </ButtonLink>
  )
}

// A stable default: a fresh `{}` per render would defeat memoisation of anything keyed on `filter`.
const NO_FILTER: AuditFilter = {}

// AuditLogSection is the Enterprise Audit log of the workspace: who changed what and
// when, newest first, narrowed by `filter`. It renders nothing for a plain member or
// without a license.
export function AuditLogSection({
  slug,
  filter = NO_FILTER,
  onFilterChange,
}: {
  slug: string
  filter?: AuditFilter
  onFilterChange: (filter: AuditFilter) => void
}) {
  const { t } = useTranslation()
  const probe = useQuery(siteAuditListOptions({ path: { slug }, query: { limit: 1 } }))
  if (isHidden(probe.error)) {
    return null
  }
  const filterKey = JSON.stringify(filter)

  return (
    <Card withBorder>
      <Stack>
        <Group justify="space-between">
          <Title order={4}>{t(($) => $.settings.auditLog.title)}</Title>
          <ExportButton slug={slug} filter={filter} />
        </Group>
        <RetentionControl slug={slug} />
        <FilterForm key={filterKey} filter={filter} onChange={onFilterChange} />
        <AuditPage key={filterKey} slug={slug} filter={filter} first />
      </Stack>
    </Card>
  )
}
