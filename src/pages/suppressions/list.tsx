import {
  Box,
  EmptyState,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  Stack,
  Switch,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useCounter, useDisclosure } from '@mantine/hooks'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useDeferredValue, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSuppressionsCreateMutation,
  siteSuppressionsDeleteMutation,
  siteSuppressionsListOptions,
  siteSuppressionsListQueryKey,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSuppressionReason } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as suppressionsRoute } from '../../routes/workspaces/$workspaceId/suppressions/index.tsx'
import { can } from '../../utils/permissions.ts'

const REASON_COLORS: Record<SiteSuppressionReason, string> = {
  bounce: 'orange',
  complaint: 'red',
  manual: 'gray',
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
}: {
  workspaceId: string
  page: number
  lifted: boolean
  canManage: boolean
  fetching: boolean
  onPageChange: (page: number) => void
  onLift: (id: string) => void
}) {
  const { t } = useTranslation()
  const { items: records, totalItems } = useSuspenseQuery(
    siteSuppressionsListOptions({
      path: { workspaceId },
      query: { page, pageSize: PAGE_SIZE, lifted },
    }),
  ).data

  return (
    <DataTable
      minHeight={records.length === 0 ? 200 : 0}
      withTableBorder
      records={records}
      idAccessor="id"
      fetching={fetching}
      columns={[
        { accessor: 'destination', title: t(($) => $.suppressions.emailColumn) },
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
          render: (record) =>
            record.liftedAt ? (
              <Badge variant="light" color="gray">
                {t(($) => $.suppressions.liftedBadge)}
              </Badge>
            ) : canManage ? (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <Menu.Item onClick={() => onLift(record.id)}>
                    {t(($) => $.suppressions.lift)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ) : null,
        },
      ]}
      emptyState={<EmptyState title={t(($) => $.suppressions.empty)} />}
      totalRecords={totalItems}
      recordsPerPage={PAGE_SIZE}
      page={page}
      onPageChange={onPageChange}
    />
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

  const queryKey = siteSuppressionsListQueryKey({ path: { workspaceId } })

  const form = useForm<{ destination: string }>({ initialValues: { destination: '' } })

  const createMutation = useResourceMutation({
    mutation: siteSuppressionsCreateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.suppressions.createError),
    onDone: () => {
      form.reset()
      closeCreate()
      pageHandlers.set(1) // newest is first (id desc), so jump to page 1 to show it
    },
  })

  const liftMutation = useResourceMutation({
    mutation: siteSuppressionsDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.suppressions.liftError),
  })

  const onLift = (id: string) =>
    confirmLift({
      title: t(($) => $.suppressions.liftConfirmTitle),
      description: t(($) => $.suppressions.liftConfirmDescription),
      confirmLabel: t(($) => $.suppressions.lift),
      onConfirm: () => liftMutation.mutate({ path: { workspaceId, id } }),
    })

  return (
    <Box>
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
        />
      </QueryBoundary>
      <Modal opened={createOpened} onClose={closeCreate} title={t(($) => $.suppressions.create)}>
        <form
          onSubmit={form.onSubmit((values) =>
            createMutation.mutate({
              path: { workspaceId },
              body: { destination: values.destination.trim() },
            }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.suppressions.emailLabel)}
              description={t(($) => $.suppressions.emailHint)}
              placeholder="blocked@example.com"
              type="email"
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
    </Box>
  )
}
