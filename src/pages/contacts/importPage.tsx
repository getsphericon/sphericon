import {
  Alert,
  Badge,
  Button,
  Checkbox,
  Group,
  Progress,
  Select,
  SimpleGrid,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { LeaveDialog } from '../../components/LeaveDialog.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { PageSkeleton } from '../../components/Skeletons.tsx'
import {
  siteContactImportsCancelMutation,
  siteContactImportsGetOptions,
  siteContactImportsGetQueryKey,
  siteContactImportsPreviewOptions,
  siteContactImportsPreviewQueryKey,
  siteContactImportsReportLinkMutation,
  siteContactImportsRunMutation,
  siteContactImportsSaveMappingMutation,
  siteCustomFieldsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  ContactImportAttribute,
  ContactImportDateFormat,
  ContactImportIssue,
  ContactImportKey,
  ContactImportState,
  SiteContactImport,
  SiteContactImportPreview,
} from '../../generated/site/types.gen.ts'
import {
  zContactImportDateFormat,
  zContactImportDecimalSeparator,
} from '../../generated/site/zod.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as importPageRoute } from '../../routes/workspaces/$workspaceId/contacts/imports/$importId.tsx'
import { countryOptions } from '../../utils/countries.ts'
import {
  columnsMissingDateFormat,
  ignore,
  mappingInputOf,
  tags,
  targets,
  valuesOf,
  writesDate,
} from './importMapping.ts'
import type { FieldOption, MappingValues } from './importMapping.ts'

// A CSV import's page (ADR 0138): the mapping of its columns and its preview before it runs, its
// progress while it runs, and what it wrote with its refused rows' report after.
export function ContactImportPage() {
  const { t } = useTranslation()
  const { workspaceId, importId } = importPageRoute.useParams()
  return (
    <QueryBoundary fallback={<PageSkeleton />} errorTitle={t(($) => $.contactImports.loadError)}>
      <ImportView workspaceId={workspaceId} importId={importId} />
    </QueryBoundary>
  )
}

function ImportView({ workspaceId, importId }: { workspaceId: string; importId: string }) {
  const { t } = useTranslation()
  // A running import is read again every two seconds, to show its progress.
  const { data: imp } = useSuspenseQuery({
    ...siteContactImportsGetOptions({ path: { workspaceId, importId } }),
    refetchInterval: (query) => (query.state.data?.state === 'running' ? 2000 : false),
  })
  const editable = imp.state === 'uploaded' || imp.state === 'ready'
  return (
    <Stack flex={1}>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.contactImports.title)}</Title>
        <StateBadge state={imp.state} />
      </Group>
      <Text c="dimmed">
        {t(($) => $.contactImports.fileFacts, { rows: imp.rows, columns: imp.header.length })}
      </Text>
      {imp.malformedRows > 0 ? (
        <Alert color="yellow">
          {t(($) => $.contactImports.malformedRows, { count: imp.malformedRows })}
        </Alert>
      ) : null}
      {editable ? (
        <MappingForm workspaceId={workspaceId} imp={imp} />
      ) : (
        <RunView workspaceId={workspaceId} imp={imp} />
      )}
    </Stack>
  )
}

function StateBadge({ state }: { state: ContactImportState }) {
  const { t } = useTranslation()
  const labels: Record<ContactImportState, string> = {
    uploaded: t(($) => $.contactImports.states.uploaded),
    ready: t(($) => $.contactImports.states.ready),
    running: t(($) => $.contactImports.states.running),
    cancelled: t(($) => $.contactImports.states.cancelled),
    failed: t(($) => $.contactImports.states.failed),
    done: t(($) => $.contactImports.states.done),
  }
  return <Badge variant="light">{labels[state]}</Badge>
}

// The labels of what a column writes: the alias keys and core attributes by name.
function useTargetLabels() {
  const { t } = useTranslation()
  const keys: Record<ContactImportKey, string> = {
    subjectId: t(($) => $.contactImports.keys.subjectId),
    email: t(($) => $.contactImports.keys.email),
    phone: t(($) => $.contactImports.keys.phone),
  }
  const attributes: Record<ContactImportAttribute, string> = {
    firstName: t(($) => $.contactImports.attributes.firstName),
    lastName: t(($) => $.contactImports.attributes.lastName),
    timeZone: t(($) => $.contactImports.attributes.timeZone),
    country: t(($) => $.contactImports.attributes.country),
    locale: t(($) => $.contactImports.attributes.locale),
    city: t(($) => $.contactImports.attributes.city),
    avatar: t(($) => $.contactImports.attributes.avatar),
    birthday: t(($) => $.contactImports.attributes.birthday),
  }
  return { keys, attributes }
}

// The label of a date format: the format itself, a date and time named in words.
function dateFormatLabel(format: ContactImportDateFormat, dateTime: string): string {
  return format === 'rfc3339' ? dateTime : format
}

function MappingForm({ workspaceId, imp }: { workspaceId: string; imp: SiteContactImport }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const labels = useTargetLabels()
  const { data: catalogue } = useSuspenseQuery(
    siteCustomFieldsListOptions({ path: { workspaceId } }),
  )
  const fields: FieldOption[] = catalogue.items.map((field) => ({
    key: field.key,
    type: field.type,
  }))
  const [values, setValues] = useState<MappingValues>(() => valuesOf(imp.mapping))
  const [previewed, setPreviewed] = useState(false)
  // Edits not saved yet, which leaving the page would lose.
  const [dirty, setDirty] = useState(false)
  const path = { workspaceId, importId: imp.id }
  const refresh = () =>
    void queryClient.invalidateQueries({ queryKey: siteContactImportsGetQueryKey({ path }) })
  const save = useMutation({
    ...siteContactImportsSaveMappingMutation(),
    onSuccess: () => {
      setPreviewed(true)
      setDirty(false)
      void queryClient.invalidateQueries({ queryKey: siteContactImportsPreviewQueryKey({ path }) })
    },
  })
  const run = useMutation({ ...siteContactImportsRunMutation(), onSuccess: refresh })
  const preview = useQuery({
    ...siteContactImportsPreviewOptions({ path }),
    enabled: previewed && save.isSuccess,
  })
  const missing = columnsMissingDateFormat(values, fields)
  const all = targets(fields)
  const targetData = [
    { value: ignore, label: t(($) => $.contactImports.ignore) },
    { value: tags, label: t(($) => $.contactImports.tags) },
    {
      group: t(($) => $.contactImports.keysGroup),
      items: all.keys.map(({ value, key }) => ({ value, label: labels.keys[key] })),
    },
    {
      group: t(($) => $.contactImports.attributesGroup),
      items: all.attributes.map(({ value, attribute }) => ({
        value,
        label: labels.attributes[attribute],
      })),
    },
    {
      group: t(($) => $.contactImports.customFieldsGroup),
      items: all.customFields.map(({ value, key }) => ({ value, label: key })),
    },
  ]
  const dateFormats = zContactImportDateFormat.options.map((format) => ({
    value: format,
    label: dateFormatLabel(
      format,
      t(($) => $.contactImports.dateTime),
    ),
  }))
  const setColumn = (at: number, patch: Partial<MappingValues['columns'][number]>) => {
    setDirty(true)
    setValues((prev) => ({
      ...prev,
      columns: prev.columns.map((choice, i) => (i === at ? { ...choice, ...patch } : choice)),
    }))
  }
  const edit = (patch: Partial<MappingValues>) => {
    setDirty(true)
    setValues((prev) => ({ ...prev, ...patch }))
    setPreviewed(false)
  }

  return (
    <Stack>
      <LeaveDialog dirty={dirty} message={t(($) => $.contactImports.leaveMessage)} />
      <Title order={3}>{t(($) => $.contactImports.mappingTitle)}</Title>
      <Table striped>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t(($) => $.contactImports.column)}</Table.Th>
            <Table.Th>{t(($) => $.contactImports.writes)}</Table.Th>
            <Table.Th>{t(($) => $.contactImports.dateFormat)}</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {imp.header.map((header, at) => {
            const choice = values.columns[at] ?? { target: ignore, dateFormat: null }
            return (
              <Table.Tr key={at}>
                <Table.Td>{header === '' ? '-' : header}</Table.Td>
                <Table.Td>
                  <Select
                    aria-label={t(($) => $.contactImports.writesColumn, { column: header })}
                    data={targetData}
                    value={choice.target}
                    allowDeselect={false}
                    searchable
                    onChange={(target) => {
                      setColumn(at, { target: target ?? ignore })
                      setPreviewed(false)
                    }}
                  />
                </Table.Td>
                <Table.Td>
                  {writesDate(choice.target, fields) ? (
                    <Select
                      aria-label={t(($) => $.contactImports.dateFormatColumn, { column: header })}
                      placeholder={t(($) => $.contactImports.chooseDateFormat)}
                      data={dateFormats}
                      value={choice.dateFormat}
                      onChange={(format) => {
                        setColumn(at, {
                          dateFormat: zContactImportDateFormat.nullable().parse(format),
                        })
                        setPreviewed(false)
                      }}
                    />
                  ) : (
                    '-'
                  )}
                </Table.Td>
              </Table.Tr>
            )
          })}
        </Table.Tbody>
      </Table>

      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <Select
          label={t(($) => $.contactImports.decimalSeparator)}
          data={zContactImportDecimalSeparator.options.map((sep) => ({
            value: sep,
            label:
              sep === '.' ? t(($) => $.contactImports.point) : t(($) => $.contactImports.comma),
          }))}
          value={values.decimalSeparator}
          allowDeselect={false}
          onChange={(sep) => edit({ decimalSeparator: zContactImportDecimalSeparator.parse(sep) })}
        />
        <Select
          label={t(($) => $.contactImports.defaultCountry)}
          description={t(($) => $.contactImports.defaultCountryHint)}
          data={countryOptions}
          value={values.defaultCountry}
          searchable
          clearable
          onChange={(country) => edit({ defaultCountry: country })}
        />
        <TextInput
          label={t(($) => $.contactImports.tag)}
          description={t(($) => $.contactImports.tagHint)}
          value={values.tag}
          onChange={(event) => edit({ tag: event.currentTarget.value })}
        />
        <Checkbox
          mt="lg"
          label={t(($) => $.contactImports.onlyAddNew)}
          description={t(($) => $.contactImports.onlyAddNewHint)}
          checked={values.onlyAddNew}
          onChange={(event) => edit({ onlyAddNew: event.currentTarget.checked })}
        />
      </SimpleGrid>

      {save.isError ? (
        <ApiErrorAlert error={save.error} title={t(($) => $.contactImports.saveError)} />
      ) : null}
      {run.isError ? (
        <ApiErrorAlert error={run.error} title={t(($) => $.contactImports.runError)} />
      ) : null}
      {missing.length > 0 ? (
        <Text c="dimmed">{t(($) => $.contactImports.runNeedsDateFormats)}</Text>
      ) : null}
      <Group>
        <Button
          variant="default"
          loading={save.isPending}
          onClick={() => save.mutate({ path, body: mappingInputOf(values, fields) })}
        >
          {t(($) => $.contactImports.preview)}
        </Button>
        <Button
          disabled={missing.length > 0 || !previewed}
          loading={run.isPending}
          onClick={() => run.mutate({ path })}
        >
          {t(($) => $.contactImports.run)}
        </Button>
      </Group>
      {preview.isError ? (
        <ApiErrorAlert error={preview.error} title={t(($) => $.contactImports.previewError)} />
      ) : null}
      {previewed && preview.data !== undefined ? (
        <PreviewView header={imp.header} preview={preview.data} />
      ) : null}
    </Stack>
  )
}

// What a row's issues say, joined: why it is refused, and each value it drops.
function issuesText(issues: ContactImportIssue[]): string {
  return issues.map((issue) => issue.message).join('; ')
}

function PreviewView({ header, preview }: { header: string[]; preview: SiteContactImportPreview }) {
  const { t } = useTranslation()
  return (
    <Stack>
      <Title order={3}>{t(($) => $.contactImports.previewTitle)}</Title>
      <Text>
        {t(($) => $.contactImports.previewTotals, {
          rows: preview.totalRows,
          withoutKey: preview.rowsWithoutKey,
          malformed: preview.malformedRows,
          tags: preview.distinctTags,
        })}
      </Text>
      <Table.ScrollContainer minWidth={600}>
        <Table striped>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.contactImports.row)}</Table.Th>
              {header.map((name, at) => (
                <Table.Th key={at}>{name}</Table.Th>
              ))}
              <Table.Th>{t(($) => $.contactImports.issues)}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {preview.rows.map((row) => (
              <Table.Tr key={row.row}>
                <Table.Td>{row.row}</Table.Td>
                {header.map((_, at) => (
                  <Table.Td key={at}>{row.cells[at] ?? ''}</Table.Td>
                ))}
                <Table.Td>
                  <Stack gap={2}>
                    {row.refusal !== null ? (
                      <Text size="sm" c="red">
                        {row.refusal.message}
                      </Text>
                    ) : null}
                    {row.dropped.length > 0 ? (
                      <Text size="sm">{issuesText(row.dropped)}</Text>
                    ) : null}
                    {row.warnings.length > 0 ? (
                      <Text size="sm" c="dimmed">
                        {t(($) => $.contactImports.addressWarning)}
                      </Text>
                    ) : null}
                  </Stack>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}

function RunView({ workspaceId, imp }: { workspaceId: string; imp: SiteContactImport }) {
  const { t } = useTranslation()
  const link = useMutation({
    ...siteContactImportsReportLinkMutation(),
    // The browser follows the link, which streams the report to disk.
    onSuccess: (download) => window.location.assign(download.url),
  })
  const counts = imp.counts
  return (
    <Stack>
      {imp.state === 'running' ? <RunProgress workspaceId={workspaceId} imp={imp} /> : null}
      {imp.state === 'cancelled' ? (
        <Alert color="yellow">{t(($) => $.contactImports.cancelled)}</Alert>
      ) : null}
      {imp.state === 'failed' ? (
        <Alert color="red">{t(($) => $.contactImports.failed)}</Alert>
      ) : null}
      <Text>
        {t(($) => $.contactImports.counts, {
          created: counts.created,
          updated: counts.updated,
          refused: counts.refused,
          dropped: counts.dropped,
        })}
      </Text>
      {imp.reportRows > 0 && imp.state !== 'running' ? (
        <Group>
          <Button
            variant="default"
            loading={link.isPending}
            onClick={() => link.mutate({ path: { workspaceId, importId: imp.id } })}
          >
            {t(($) => $.contactImports.downloadReport)}
          </Button>
          <Text size="sm" c="dimmed">
            {t(($) => $.contactImports.reportHint, { count: imp.reportRows })}
          </Text>
        </Group>
      ) : null}
      {link.isError ? (
        <ApiErrorAlert error={link.error} title={t(($) => $.contactImports.reportError)} />
      ) : null}
    </Stack>
  )
}

// A running import's progress, kept by its job across restarts, and the Cancel that stops it after
// the batch it is writing.
function RunProgress({ workspaceId, imp }: { workspaceId: string; imp: SiteContactImport }) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const cancel = useResourceMutation({
    mutation: siteContactImportsCancelMutation(),
    errorTitle: t(($) => $.contactImports.cancelError),
  })
  const stopping = imp.cancelRequestedAt !== null
  const counts = imp.counts
  return (
    <Stack gap={4}>
      <Progress value={imp.rows === 0 ? 0 : (100 * counts.processed) / imp.rows} />
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          {stopping
            ? t(($) => $.contactImports.stopping)
            : t(($) => $.contactImports.progress, { processed: counts.processed, rows: imp.rows })}
        </Text>
        <Button
          variant="default"
          color="red"
          disabled={stopping}
          loading={cancel.isPending}
          onClick={() =>
            confirm({
              title: t(($) => $.contactImports.cancelTitle),
              description: t(($) => $.contactImports.cancelDescription),
              confirmLabel: t(($) => $.contactImports.cancelConfirm),
              cancelLabel: t(($) => $.contactImports.cancelKeep),
              onConfirm: () => cancel.mutate({ path: { workspaceId, importId: imp.id } }),
            })
          }
        >
          {t(($) => $.contactImports.cancel)}
        </Button>
      </Group>
    </Stack>
  )
}
