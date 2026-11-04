import {
  EmptyState,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  NativeSelect,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useCounter, useDisclosure } from '@mantine/hooks'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { DataTable } from 'mantine-datatable'
import { useDeferredValue, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredEmptyState } from '../../components/CenteredEmptyState.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSuppressionsCreateMutation,
  siteSuppressionsDeleteMutation,
  siteSuppressionsListOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteCreateSuppressionInput,
  SiteSuppressionChannel,
  SiteSuppressionReason,
} from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { usePaginationText } from '../../hooks/usePaginationText.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as suppressionsRoute } from '../../routes/workspaces/$workspaceId/suppressions/index.tsx'
import { can } from '../../utils/permissions.ts'
import { isErasedNumber } from '../../utils/phone.ts'

const REASON_COLORS: Record<SiteSuppressionReason, string> = {
  bounce: 'orange',
  complaint: 'red',
  manual: 'gray',
}

// What a destination is on each Channel, as the add form asks for it.
const destinationField: Record<
  SiteSuppressionChannel,
  {
    label: (t: TFunction) => string
    hint: (t: TFunction) => string
    placeholder: string
    type: 'email' | 'text' | 'tel'
  }
> = {
  email: {
    label: (t) => t(($) => $.suppressions.emailLabel),
    hint: (t) => t(($) => $.suppressions.emailHint),
    placeholder: 'blocked@example.com',
    type: 'email',
  },
  telegram: {
    label: (t) => t(($) => $.suppressions.telegramLabel),
    hint: (t) => t(($) => $.suppressions.telegramHint),
    placeholder: '123456789',
    type: 'text',
  },
  sms: {
    label: (t) => t(($) => $.suppressions.smsLabel),
    hint: (t) => t(($) => $.suppressions.smsHint),
    placeholder: '+14155550132',
    type: 'tel',
  },
}

// The list grows automatically (bounces/complaints), so it is paginated
// server-side rather than rendering only the first page.
const PAGE_SIZE = 20

// SuppressionsTable is one page of the list; the page's boundary shows its first load and a
// failure.
function SuppressionsTable({
  workspaceId,
  page,
  lifted,
  canManage,
  fetching,
  onPageChange,
  onLift,
  onCreate,
}: {
  workspaceId: string
  page: number
  lifted: boolean
  canManage: boolean
  fetching: boolean
  onPageChange: (page: number) => void
  onLift: (id: string) => void
  onCreate: () => void
}) {
  const { t } = useTranslation()
  const paginationText = usePaginationText()
  const { items: records, totalItems } = useSuspenseQuery(
    siteSuppressionsListOptions({
      path: { workspaceId },
      query: { page, pageSize: PAGE_SIZE, lifted },
    }),
  ).data

  return (
    // It grows into the page's free height, so an empty list's state sits in the page's middle.
    <Stack gap="xs" flex={1}>
      <Text fw={600}>
        {t(($) => $.table.collection, { name: t(($) => $.suppressions.title), total: totalItems })}
      </Text>
      {/* No suppression at all (the total, not the page) shows its empty state in place of the
          table, centered in the page. */}
      {totalItems === 0 ? (
        <CenteredEmptyState title={t(($) => $.suppressions.empty)}>
          {canManage ? (
            <EmptyState.Actions>
              <Button variant="default" onClick={onCreate}>
                {t(($) => $.suppressions.create)}
              </Button>
            </EmptyState.Actions>
          ) : null}
        </CenteredEmptyState>
      ) : (
        <DataTable
          withTableBorder
          records={records}
          idAccessor="id"
          fetching={fetching}
          columns={[
            {
              accessor: 'destination',
              title: t(($) => $.suppressions.emailColumn),
              render: (record) =>
                record.channel === 'sms' && isErasedNumber(record.destination) ? (
                  <Text size="sm" c="dimmed" fs="italic">
                    {t(($) => $.suppressions.erasedNumber)}
                  </Text>
                ) : (
                  record.destination
                ),
            },
            {
              accessor: 'reason',
              title: t(($) => $.suppressions.reasonColumn),
              render: (record) => (
                <Badge variant="light" color={REASON_COLORS[record.reason]}>
                  {t(($) => $.suppressions.reasons[record.reason])}
                </Badge>
              ),
            },
            {
              accessor: 'actions',
              title: '',
              render: (record) => {
                if (record.liftedAt) {
                  return (
                    <Badge variant="light" color="gray">
                      {t(($) => $.suppressions.liftedBadge)}
                    </Badge>
                  )
                }
                if (!canManage) {
                  return null
                }
                return (
                  <Group justify="flex-end">
                    <RowActionsMenu>
                      <Menu.Item onClick={() => onLift(record.id)}>
                        {t(($) => $.suppressions.lift)}
                      </Menu.Item>
                    </RowActionsMenu>
                  </Group>
                )
              },
            },
          ]}
          totalRecords={totalItems}
          paginationText={paginationText}
          recordsPerPage={PAGE_SIZE}
          page={page}
          onPageChange={onPageChange}
        />
      )}
    </Stack>
  )
}

// SuppressionsListPage manages the workspace do-not-send list: hard bounces and
// complaints land here automatically; destinations can also be added manually.
// The send path skips every active destination listed here; a lifted one stays as a record
// and is shown on request. (Unsubscribes are a
// separate, per-source opt-out, not a suppression.) Every member reads the list; adding and
// lifting are offered only with suppressions:manage, as the API refuses everyone else.
export function SuppressionsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = suppressionsRoute.useParams()
  const confirmLift = useConfirmation()
  const [page, pageHandlers] = useCounter(1, { min: 1 })
  const [lifted, setLifted] = useState(false)
  // The table reads deferred copies, so while another page or the lifted list loads the table
  // it replaces stays up under its loader instead of falling back to the skeleton, and the
  // pager and the switch answer at once.
  const shownPage = useDeferredValue(page)
  const shownLifted = useDeferredValue(lifted)
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
  const workspaces = useQuery(siteWorkspacesListOptions())
  const canManage = can(
    workspaces.data?.find((w) => w.id === workspaceId),
    'suppressions:manage',
  )

  const form = useForm<Required<Pick<SiteCreateSuppressionInput, 'channel' | 'destination'>>>({
    initialValues: { channel: 'email', destination: '' },
  })

  const createMutation = useResourceMutation({
    mutation: siteSuppressionsCreateMutation(),
    errorTitle: t(($) => $.suppressions.createError),
    onDone: () => {
      form.reset()
      closeCreate()
      pageHandlers.set(1) // newest is first (id desc), so jump to page 1 to show it
    },
  })

  const liftMutation = useResourceMutation({
    mutation: siteSuppressionsDeleteMutation(),
    errorTitle: t(($) => $.suppressions.liftError),
  })

  const onLift = (id: string) =>
    confirmLift({
      title: t(($) => $.suppressions.liftConfirmTitle),
      description: t(($) => $.suppressions.liftConfirmDescription),
      confirmLabel: t(($) => $.suppressions.lift),
      onConfirm: () => liftMutation.mutate({ path: { workspaceId, id } }),
    })

  // One option per Channel, in the order a person reads them.
  const channelLabels: Record<SiteSuppressionChannel, string> = {
    email: t(($) => $.suppressions.channelEmail),
    telegram: t(($) => $.suppressions.channelTelegram),
    sms: t(($) => $.suppressions.channelSms),
  }

  return (
    <Stack gap={0} flex={1}>
      <Group justify="space-between" mb="sm">
        <Title order={2}>{t(($) => $.suppressions.title)}</Title>
        {canManage ? (
          <Button size="xs" onClick={openCreate}>
            {t(($) => $.suppressions.create)}
          </Button>
        ) : null}
      </Group>

      <Switch
        mb="sm"
        label={t(($) => $.suppressions.showLifted)}
        checked={lifted}
        onChange={(event) => {
          setLifted(event.currentTarget.checked)
          pageHandlers.set(1)
        }}
      />

      <QueryBoundary fallback={<TableSkeleton />} errorTitle={t(($) => $.suppressions.loadError)}>
        <SuppressionsTable
          workspaceId={workspaceId}
          page={shownPage}
          lifted={shownLifted}
          canManage={canManage}
          fetching={page !== shownPage || lifted !== shownLifted}
          onPageChange={pageHandlers.set}
          onLift={onLift}
          onCreate={openCreate}
        />
      </QueryBoundary>
      <Modal opened={createOpened} onClose={closeCreate} title={t(($) => $.suppressions.create)}>
        <form
          onSubmit={form.onSubmit((values) =>
            createMutation.mutate({
              path: { workspaceId },
              body: { channel: values.channel, destination: values.destination.trim() },
            }),
          )}
        >
          <Stack>
            <NativeSelect
              label={t(($) => $.suppressions.channelLabel)}
              data={Object.entries(channelLabels).map(([value, label]) => ({ value, label }))}
              {...form.getInputProps('channel')}
            />
            <TextInput
              label={destinationField[form.values.channel].label(t)}
              description={destinationField[form.values.channel].hint(t)}
              placeholder={destinationField[form.values.channel].placeholder}
              type={destinationField[form.values.channel].type}
              required
              data-autofocus
              {...form.getInputProps('destination')}
            />
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.suppressions.create)}
            </Button>
          </Stack>
        </form>
      </Modal>
    </Stack>
  )
}
