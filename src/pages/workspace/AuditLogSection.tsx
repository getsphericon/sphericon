import {
  Anchor,
  Box,
  EmptyState,
  ActionIcon,
  Badge,
  Button,
  CloseButton,
  Code,
  Group,
  Modal,
  NumberInput,
  Select,
  SimpleGrid,
  Stack,
  Table,
  Text,
} from '@mantine/core'
import { DatePickerInput } from '@mantine/dates'
import type { DatesRangeValue } from '@mantine/dates'
import { useDisclosure } from '@mantine/hooks'
import { IconChevronDown, IconChevronRight, IconDownload } from '@tabler/icons-react'
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { Fragment, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { ActionOption } from '../../components/ActionName.tsx'
import { AuditActionName } from '../../components/AuditActionName.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteAuditExportOptions,
  siteAuditFacetsOptions,
  siteAuditGetRetentionOptions,
  siteAuditListOptions,
  siteAuditSetRetentionMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteAuditActor,
  SiteAuditActorKind,
  SiteAuditEntryResource,
} from '../../generated/site/types.gen.ts'
import { zRetentionDays, zSiteAuditActorKind } from '../../generated/site/zod.gen.ts'
import { useAuditActionLabel } from '../../hooks/useAuditActionLabel.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { Route as auditLogRoute } from '../../routes/workspaces/$workspaceId/settings/audit-log.tsx'
import { labelledActionFilter } from '../../utils/actionFilter.ts'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import {
  actorValue,
  auditQuery,
  filterActorValue,
  filterChips,
  withAction,
  withActor,
  withPeriod,
  withValue,
} from '../../utils/auditFilter.ts'
import type { AuditChipField, AuditFilter } from '../../utils/auditFilter.ts'
import { boundsOf } from '../../utils/bounds.ts'
import { dayOf, daysBefore } from '../../utils/datetime.ts'

const PAGE_SIZE = 25
const RETENTION_DAYS = boundsOf(zRetentionDays)
// The API answers 402 without an Enterprise license and 403 to a plain member; in both
// cases the section is simply not offered.
const HIDDEN_STATUSES = [402, 403]

function isHidden(error: unknown) {
  return hasErrorStatus(error, HIDDEN_STATUSES)
}

// useActorLabel names an actor: the system as such, an API token by its name or its id,
// anyone else by the name snapshot of the entry.
function useActorLabel() {
  const { t } = useTranslation()
  return ({ kind, name, id }: SiteAuditActor) => {
    if (kind === 'system') {
      return t(($) => $.settings.auditLog.actorSystem)
    }
    if (kind === 'api_token') {
      return name ?? t(($) => $.settings.auditLog.actorToken, { id: id ?? '' })
    }
    return name ?? id ?? ''
  }
}

interface FilterProps {
  filter: AuditFilter
  onFilterChange: (filter: AuditFilter) => void
}

// FilterValue is a value of an entry that narrows the log to the entries sharing it.
function FilterValue({
  next,
  onFilterChange,
  children,
}: {
  next: AuditFilter
  onFilterChange: (filter: AuditFilter) => void
  children: ReactNode
}) {
  return (
    <Anchor component="button" type="button" size="sm" onClick={() => onFilterChange(next)}>
      {children}
    </Anchor>
  )
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

// EntryDetails is what an expanded entry shows beneath it: the changed fields, and the request
// and client address it came from, each a value that narrows the log to its entries.
function EntryDetails({
  entry,
  diff,
  filter,
  onFilterChange,
}: FilterProps & { entry: SiteAuditEntryResource; diff: Record<string, unknown> | null }) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      {diff ? <DiffView diff={diff} /> : null}
      <Group gap="lg">
        {entry.requestId ? (
          <Group gap={4}>
            <Text span c="dimmed" size="sm">
              {t(($) => $.settings.auditLog.requestId)}
            </Text>
            <FilterValue
              next={withValue(filter, 'requestId', entry.requestId)}
              onFilterChange={onFilterChange}
            >
              {entry.requestId}
            </FilterValue>
          </Group>
        ) : null}
        {entry.ip ? (
          <Group gap={4}>
            <Text span c="dimmed" size="sm">
              {t(($) => $.settings.auditLog.ip)}
            </Text>
            <FilterValue next={withValue(filter, 'ip', entry.ip)} onFilterChange={onFilterChange}>
              {entry.ip}
            </FilterValue>
          </Group>
        ) : null}
      </Group>
    </Stack>
  )
}

// The filter that narrows the log to an entry's target: its type, and its id when it has one.
function targetFilter(filter: AuditFilter, { type, id }: SiteAuditEntryResource['target']) {
  return withValue(withValue(filter, 'targetType', type), 'targetId', id)
}

// EntryRows is one entry; when it has details (a diff, a request, a client address), a chevron
// expands them beneath the row so the list stays scannable. Its actor and target narrow the
// log to their entries.
function EntryRows({
  entry,
  filter,
  onFilterChange,
}: FilterProps & { entry: SiteAuditEntryResource }) {
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  const actorLabel = useActorLabel()
  const [open, { toggle }] = useDisclosure(false)
  const diff = entry.diff && Object.keys(entry.diff).length > 0 ? entry.diff : null
  const expandable = Boolean(diff ?? entry.requestId ?? entry.ip)

  return (
    <Fragment>
      <Table.Tr>
        <Table.Td>{formatDateTime(entry.occurredAt)}</Table.Td>
        <Table.Td>
          <FilterValue
            next={withActor(filter, actorValue(entry.actor))}
            onFilterChange={onFilterChange}
          >
            {actorLabel(entry.actor)}
          </FilterValue>
        </Table.Td>
        <Table.Td>
          <AuditActionName action={entry.action} />
        </Table.Td>
        <Table.Td>
          <FilterValue next={targetFilter(filter, entry.target)} onFilterChange={onFilterChange}>
            <TargetLabel entry={entry} />
          </FilterValue>
        </Table.Td>
        <Table.Td>
          {expandable ? (
            <ActionIcon
              variant="subtle"
              color="gray"
              aria-expanded={open}
              aria-label={
                open ? t(($) => $.settings.auditLog.collapse) : t(($) => $.settings.auditLog.expand)
              }
              onClick={toggle}
            >
              {open ? <IconChevronDown size={16} /> : <IconChevronRight size={16} />}
            </ActionIcon>
          ) : null}
        </Table.Td>
      </Table.Tr>
      {expandable && open ? (
        <Table.Tr>
          <Table.Td colSpan={5}>
            <EntryDetails
              entry={entry}
              diff={diff}
              filter={filter}
              onFilterChange={onFilterChange}
            />
          </Table.Td>
        </Table.Tr>
      ) : null}
    </Fragment>
  )
}

interface AuditPageProps extends FilterProps {
  workspaceId: string
  cursor?: string
  first: boolean
}

// AuditPage is one page of the log under a boundary of its own, so a next page loads and fails
// beneath the rows already shown; a refused log shows nothing.
function AuditPage(props: AuditPageProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={<BlockSkeleton />}
      errorTitle={t(($) => $.settings.auditLog.loadError)}
      renderError={(error) => (isHidden(error) ? null : undefined)}
    >
      <AuditPageRows {...props} />
    </QueryBoundary>
  )
}

// AuditPageRows renders one page of the log and, while more exist, a button that mounts the
// next page beneath it, so the list grows without holding every page in one state.
function AuditPageRows({ workspaceId, filter, onFilterChange, cursor, first }: AuditPageProps) {
  const { t } = useTranslation()
  const [showNext, setShowNext] = useState(false)
  const { items, nextCursor } = useSuspenseQuery(
    siteAuditListOptions({
      path: { workspaceId },
      query: { ...auditQuery(filter), limit: PAGE_SIZE, ...(cursor ? { cursor } : {}) },
    }),
  ).data

  const next = nextCursor ?? undefined
  if (first && items.length === 0) {
    return (
      <EmptyState
        title={
          Object.keys(auditQuery(filter)).length > 0
            ? t(($) => $.settings.auditLog.noMatches)
            : t(($) => $.settings.auditLog.empty)
        }
      />
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
              <EntryRows
                key={entry.id}
                entry={entry}
                filter={filter}
                onFilterChange={onFilterChange}
              />
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
        <AuditPage
          workspaceId={workspaceId}
          filter={filter}
          onFilterChange={onFilterChange}
          cursor={next}
          first={false}
        />
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

// AuditExportButton exports the log narrowed by the filter currently applied, so the file
// matches what the page shows. It renders nothing where the log is not offered (a plain member,
// or no Enterprise license), so the page can place it beside its title.
export function AuditExportButton({
  workspaceId,
  filter,
}: {
  workspaceId: string
  filter: AuditFilter
}) {
  const { t } = useTranslation()
  const probe = useQuery(siteAuditListOptions({ path: { workspaceId }, query: { limit: 1 } }))
  const notify = useNotify()
  const queryClient = useQueryClient()
  const exportCsv = useMutation({
    mutationFn: async () =>
      queryClient.query({
        ...siteAuditExportOptions({ path: { workspaceId }, query: auditQuery(filter) }),
        staleTime: 0,
        gcTime: 0,
      }),
    onSuccess: (csv) => saveCsv(csv, `audit-log-${workspaceId}.csv`),
    onError: () =>
      notify({
        color: 'red',
        message: t(($) => $.settings.auditLog.exportError),
      }),
  })
  if (!probe.isSuccess) {
    return null
  }

  return (
    <Button
      variant="default"
      leftSection={<IconDownload size={16} />}
      loading={exportCsv.isPending}
      onClick={() => exportCsv.mutate()}
    >
      {t(($) => $.settings.auditLog.exportCsv)}
    </Button>
  )
}

// RetentionForm sets how many days the log is kept; an empty value keeps it forever. The
// change is recorded as an Audit entry by the API.
function RetentionForm({
  workspaceId,
  days,
  onClose,
}: {
  workspaceId: string
  days: number | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  // A blank NumberInput holds '': no window, the log is kept forever.
  const schema = z.object({
    retentionDays: z
      .union([zRetentionDays, z.literal('')])
      .transform((value) => (value === '' ? null : value)),
  })
  const form = useSchemaForm(schema, { retentionDays: days ?? '' })
  const mutation = useResourceMutation({
    mutation: siteAuditSetRetentionMutation(),
    successMessage: t(($) => $.settings.auditLog.retentionSaved),
    errorTitle: t(($) => $.settings.auditLog.retentionError),
    onDone: onClose,
  })

  return (
    <form
      onSubmit={form.onSubmit(({ retentionDays }) =>
        mutation.mutate({ path: { workspaceId }, body: { retentionDays } }),
      )}
    >
      <Stack>
        <NumberInput
          data-autofocus
          label={t(($) => $.settings.auditLog.retentionLabel)}
          description={t(($) => $.settings.auditLog.retentionDescription)}
          placeholder={t(($) => $.settings.auditLog.retentionPlaceholder)}
          min={RETENTION_DAYS.min}
          max={RETENTION_DAYS.max}
          allowDecimal={false}
          allowNegative={false}
          clampBehavior="none"
          {...form.getInputProps('retentionDays')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={mutation.isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// RetentionControl is the Enterprise advanced-retention window for the log: the window as it
// stands, and a form in a modal to change it. It is mounted only with audit:configure and
// renders nothing without the license.
function RetentionControl({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const query = useQuery(siteAuditGetRetentionOptions({ path: { workspaceId } }))
  if (!query.isSuccess) {
    return null
  }
  const days = query.data.retentionDays

  return (
    <Group gap="xs">
      <Text size="sm" c="dimmed">
        {days === null
          ? t(($) => $.settings.auditLog.retentionForever)
          : t(($) => $.settings.auditLog.retentionKept, { count: days })}
      </Text>
      <Button variant="subtle" size="compact-sm" onClick={open}>
        {t(($) => $.settings.auditLog.retentionChange)}
      </Button>
      <Modal opened={opened} onClose={close} title={t(($) => $.settings.auditLog.retentionTitle)}>
        <RetentionForm workspaceId={workspaceId} days={days} onClose={close} />
      </Modal>
    </Group>
  )
}

// PeriodInput picks the days the log is narrowed to. A range picked by half stays here until
// its second day is picked; it remounts when the filter's period changes.
function PeriodInput({ filter, onFilterChange }: FilterProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState<DatesRangeValue<string>>([
    filter.from ?? null,
    filter.to ?? null,
  ])
  const [today] = useState(() => dayOf(Date.now()))

  return (
    <DatePickerInput
      type="range"
      clearable
      valueFormat="L"
      label={t(($) => $.settings.auditLog.period)}
      placeholder={t(($) => $.settings.auditLog.anyTime)}
      presets={[
        { value: [today, today], label: t(($) => $.settings.auditLog.today) },
        { value: [daysBefore(today, 6), today], label: t(($) => $.settings.auditLog.last7Days) },
        { value: [daysBefore(today, 29), today], label: t(($) => $.settings.auditLog.last30Days) },
      ]}
      value={range}
      onChange={(next) => {
        setRange(next)
        const period = withPeriod(filter, next)
        if (period) {
          onFilterChange(period)
        }
      }}
    />
  )
}

// withCurrent offers the values of the log, and the filter's own value even when the log no
// longer holds it, so a select never hides a filter that is applied.
function withCurrent<T extends string>(values: T[], current: T | undefined) {
  return current && !values.includes(current) ? [current, ...values] : values
}

// FilterChips shows the single-value filters no select offers (set by a link or by clicking a
// value in an entry), each removable, and clears the whole filter.
function FilterChips({ filter, onFilterChange }: FilterProps) {
  const { t } = useTranslation()
  const chips = filterChips(filter)
  if (Object.keys(auditQuery(filter)).length === 0) {
    return null
  }
  const labels: Record<AuditChipField, (value: string) => string> = {
    actorId: (value) => t(($) => $.settings.auditLog.chipActor, { value }),
    targetId: (value) => t(($) => $.settings.auditLog.chipTarget, { value }),
    ip: (value) => t(($) => $.settings.auditLog.chipIp, { value }),
    requestId: (value) => t(($) => $.settings.auditLog.chipRequest, { value }),
  }

  return (
    <Group gap="xs">
      {chips.map(({ field, value }) => {
        const label = labels[field](value)
        return (
          <Badge
            key={field}
            variant="light"
            color="gray"
            tt="none"
            rightSection={
              <CloseButton
                size="xs"
                aria-label={t(($) => $.settings.auditLog.removeFilter, { filter: label })}
                onClick={() => onFilterChange(withValue(filter, field, null))}
              />
            }
          >
            {label}
          </Badge>
        )
      })}
      <Button size="compact-sm" variant="subtle" onClick={() => onFilterChange({})}>
        {t(($) => $.settings.auditLog.reset)}
      </Button>
    </Group>
  )
}

// FilterBar narrows the log by period, actor, action and target type, choosing among the
// values that occur in the log; every change applies at once through the route's search, the
// single owner of the filter.
function FilterBar({ workspaceId, filter, onFilterChange }: FilterProps & { workspaceId: string }) {
  const { t } = useTranslation()
  const actorLabel = useActorLabel()
  const actionLabel = useAuditActionLabel()
  const facets = useSuspenseQuery(siteAuditFacetsOptions({ path: { workspaceId } })).data
  const kindLabels: Record<SiteAuditActorKind, string> = {
    user: t(($) => $.settings.auditLog.kindUser),
    api_token: t(($) => $.settings.auditLog.kindApiToken),
    staff: t(($) => $.settings.auditLog.kindStaff),
    system: t(($) => $.settings.auditLog.kindSystem),
  }
  const current = filterActorValue(filter)
  const actors =
    current && filter.actorKind && !facets.actors.some((actor) => actorValue(actor) === current)
      ? [...facets.actors, { kind: filter.actorKind, id: filter.actorId ?? null }]
      : facets.actors
  const actorGroups = zSiteAuditActorKind.options.flatMap((kind) => {
    const items = actors
      .filter((actor) => actor.kind === kind)
      .map((actor) => ({ value: actorValue(actor), label: actorLabel(actor) || kindLabels[kind] }))
    return items.length > 0 ? [{ group: kindLabels[kind], items }] : []
  })

  return (
    <Stack gap="xs">
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }}>
        <PeriodInput
          key={`${filter.from ?? ''}~${filter.to ?? ''}`}
          filter={filter}
          onFilterChange={onFilterChange}
        />
        <Select
          searchable
          clearable
          label={t(($) => $.settings.auditLog.actor)}
          placeholder={t(($) => $.settings.auditLog.anyActor)}
          data={actorGroups}
          value={current}
          onChange={(value) => onFilterChange(withActor(filter, value))}
        />
        <Select
          searchable
          clearable
          label={t(($) => $.settings.auditLog.action)}
          placeholder={t(($) => $.settings.auditLog.anyAction)}
          data={withCurrent(facets.actions, filter.action).map((value) => ({
            value,
            label: actionLabel(value),
          }))}
          filter={labelledActionFilter}
          renderOption={({ option }) => <ActionOption action={option.value} label={option.label} />}
          value={filter.action ?? null}
          onChange={(value) => onFilterChange(withAction(filter, value))}
        />
        <Select
          searchable
          clearable
          label={t(($) => $.settings.auditLog.targetType)}
          placeholder={t(($) => $.settings.auditLog.anyTarget)}
          data={withCurrent(facets.targetTypes, filter.targetType)}
          value={filter.targetType ?? null}
          onChange={(value) => onFilterChange(withValue(filter, 'targetType', value))}
        />
      </SimpleGrid>
      <FilterChips filter={filter} onFilterChange={onFilterChange} />
    </Stack>
  )
}

// ChangeHistoryLink opens the Audit log pre-filtered to one object (an Integration or a
// Webhook endpoint). It renders nothing where the log is not offered (a plain member, or
// no Enterprise license), so it never points at a section that is not there.
export function ChangeHistoryLink({
  workspaceId,
  targetType,
  targetId,
}: {
  workspaceId: string
  targetType: string
  targetId: string
}) {
  const { t } = useTranslation()
  const probe = useQuery(siteAuditListOptions({ path: { workspaceId }, query: { limit: 1 } }))
  if (!probe.isSuccess) {
    return null
  }

  return (
    <ButtonLink
      size="compact-sm"
      variant="subtle"
      to={auditLogRoute.to}
      params={{ workspaceId }}
      search={{ targetType, targetId }}
    >
      {t(($) => $.settings.auditLog.changeHistory)}
    </ButtonLink>
  )
}

// A stable default: a fresh `{}` per render would defeat memoisation of anything keyed on `filter`.
const NO_FILTER: AuditFilter = {}

// AuditLogSection is the Enterprise Audit log of the workspace: who changed what and
// when, newest first, narrowed by `filter`. It renders nothing without a license or when
// the API refuses the log; the retention control is offered only with `canConfigureRetention`
// (audit:configure).
export function AuditLogSection({
  workspaceId,
  filter = NO_FILTER,
  onFilterChange,
  canConfigureRetention,
}: {
  workspaceId: string
  filter?: AuditFilter
  onFilterChange: (filter: AuditFilter) => void
  canConfigureRetention: boolean
}) {
  const { t } = useTranslation()
  const probe = useQuery(siteAuditListOptions({ path: { workspaceId }, query: { limit: 1 } }))
  if (isHidden(probe.error)) {
    return null
  }
  const filterKey = JSON.stringify(filter)

  return (
    <Box>
      <Stack>
        {canConfigureRetention ? <RetentionControl workspaceId={workspaceId} /> : null}
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.settings.auditLog.loadError)}
          renderError={(error) => (isHidden(error) ? null : undefined)}
        >
          <FilterBar workspaceId={workspaceId} filter={filter} onFilterChange={onFilterChange} />
        </QueryBoundary>
        <AuditPage
          key={filterKey}
          workspaceId={workspaceId}
          filter={filter}
          onFilterChange={onFilterChange}
          first
        />
      </Stack>
    </Box>
  )
}
