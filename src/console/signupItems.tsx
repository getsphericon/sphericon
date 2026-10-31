import {
  ActionIcon,
  Badge,
  Button,
  Card,
  Checkbox,
  EmptyState,
  Group,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { IconArrowDown, IconArrowUp } from '@tabler/icons-react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import type { MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { QueryBoundary } from '../components/QueryBoundary.tsx'
import { TableSkeleton } from '../components/Skeletons.tsx'
import {
  staffSignupItemsCreateMutation,
  staffSignupItemsListOptions,
  staffSignupItemsReorderMutation,
} from '../generated/staff/@tanstack/react-query.gen.ts'
import { SignupItemKind } from '../generated/staff/types.gen.ts'
import type { StaffSignupItemResource } from '../generated/staff/types.gen.ts'
import { zSignupItemKey, zSignupItemKind, zSignupItemTitle } from '../generated/staff/zod.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { trimmed, useSchemaForm } from '../resources/schemaForm.ts'
import { Route as consoleSignupItemRoute } from '../routes/console/_authed/signup-items.$itemId.tsx'

const KINDS = zSignupItemKind.options

// Add a Sign-up item: its key (never changed, the address of its public page), title and kind,
// and whether an optional consent stands for consent to the operator's mailings.
// The kind starts unchosen (null), which the generated enum then reports as missing.
const addItemSchema = z.object({
  key: trimmed(zSignupItemKey),
  title: trimmed(zSignupItemTitle),
  kind: z.custom<SignupItemKind | null>().pipe(zSignupItemKind),
  mailingsConsent: z.boolean(),
})

function AddItemForm() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const form = useSchemaForm(addItemSchema, {
    key: '',
    title: '',
    kind: null,
    mailingsConsent: false,
  })
  const mutation = useResourceMutation({
    mutation: staffSignupItemsCreateMutation(),
    errorTitle: t(($) => $.console.signupItems.add.error),
    onDone: async (created) =>
      navigate({ to: consoleSignupItemRoute.to, params: { itemId: created.id } }),
  })
  const optional = form.values.kind === SignupItemKind.OPTIONAL_CONSENT

  return (
    <Card withBorder>
      <form
        onSubmit={form.onSubmit(({ key, title, kind, mailingsConsent }) =>
          mutation.mutate({
            body: {
              key,
              title,
              kind,
              ...(kind === SignupItemKind.OPTIONAL_CONSENT ? { mailingsConsent } : {}),
            },
          }),
        )}
      >
        <Stack>
          <Title order={4}>{t(($) => $.console.signupItems.add.title)}</Title>
          <Group grow align="flex-start" wrap="wrap">
            <TextInput
              label={t(($) => $.console.signupItems.add.keyLabel)}
              description={t(($) => $.console.signupItems.add.keyDescription)}
              miw={200}
              {...form.getInputProps('key')}
            />
            <TextInput
              label={t(($) => $.console.signupItems.add.titleLabel)}
              miw={200}
              {...form.getInputProps('title')}
            />
            <Select
              label={t(($) => $.console.signupItems.add.kindLabel)}
              data={KINDS.map((k) => ({
                value: k,
                label: t(($) => $.console.signupItems.kind[k]),
              }))}
              miw={200}
              {...form.getInputProps('kind')}
            />
          </Group>
          {optional ? (
            <Checkbox
              label={t(($) => $.console.signupItems.mailingsLabel)}
              description={t(($) => $.console.signupItems.mailingsDescription)}
              {...form.getInputProps('mailingsConsent', { type: 'checkbox' })}
            />
          ) : null}
          <Group justify="flex-end">
            <Button type="submit" loading={mutation.isPending}>
              {t(($) => $.console.signupItems.add.submit)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Card>
  )
}

// The ids of every item in their order, with the items at a and b swapped.
function swapped(items: StaffSignupItemResource[], a: number, b: number): string[] {
  const ids = items.map((item) => item.id)
  const first = ids[a]
  const second = ids[b]
  if (first === undefined || second === undefined) {
    return ids
  }
  return ids.with(a, second).with(b, first)
}

// Move an item one place up or down the sign-up form: the server takes the order of every item,
// archived ones included.
function MoveButtons({
  items,
  index,
  onMove,
  pending,
}: {
  items: StaffSignupItemResource[]
  index: number
  onMove: (ids: string[]) => void
  pending: boolean
}) {
  const { t } = useTranslation()
  const title = items[index]?.title ?? ''
  const move = (by: number) => (event: MouseEvent) => {
    // The row opens the item; a move stays on the list.
    event.stopPropagation()
    onMove(swapped(items, index, index + by))
  }

  return (
    <Group gap={4} wrap="nowrap">
      <ActionIcon
        variant="subtle"
        aria-label={t(($) => $.console.signupItems.moveUp, { title })}
        disabled={pending || index === 0}
        onClick={move(-1)}
      >
        <IconArrowUp size={16} />
      </ActionIcon>
      <ActionIcon
        variant="subtle"
        aria-label={t(($) => $.console.signupItems.moveDown, { title })}
        disabled={pending || index === items.length - 1}
        onClick={move(1)}
      >
        <IconArrowDown size={16} />
      </ActionIcon>
    </Group>
  )
}

// The Sign-up items in the order of the sign-up form; the page's boundary shows the load and its
// failure. A reorder refetches the list under the rows already shown, so the table only says it
// is fetching.
function SignupItemsTable() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const itemsQuery = useSuspenseQuery(staffSignupItemsListOptions())
  const reorder = useResourceMutation({
    mutation: staffSignupItemsReorderMutation(),
    errorTitle: t(($) => $.console.signupItems.reorderError),
  })
  const items = itemsQuery.data.items

  return (
    <DataTable
      minHeight={items.length > 0 ? 0 : 160}
      withTableBorder
      highlightOnHover
      fetching={itemsQuery.isFetching}
      records={items}
      idAccessor="id"
      columns={[
        {
          accessor: 'position',
          title: t(($) => $.console.signupItems.columns.order),
          render: (_, index) => (
            <MoveButtons
              items={items}
              index={index}
              pending={reorder.isPending}
              onMove={(ids) => reorder.mutate({ body: { ids } })}
            />
          ),
        },
        {
          accessor: 'title',
          title: t(($) => $.console.signupItems.columns.title),
          render: (record) => (
            <Group gap="xs">
              <Text size="sm">{record.title}</Text>
              {record.archivedAt ? (
                <Badge color="gray" variant="outline">
                  {t(($) => $.console.signupItems.archivedBadge)}
                </Badge>
              ) : null}
            </Group>
          ),
        },
        {
          accessor: 'key',
          title: t(($) => $.console.signupItems.columns.key),
          visibleMediaQuery: (theme) => `(min-width: ${theme.breakpoints.sm})`,
        },
        {
          accessor: 'kind',
          title: t(($) => $.console.signupItems.columns.kind),
          render: (record) => (
            <Group gap="xs">
              <Text size="sm">{t(($) => $.console.signupItems.kind[record.kind])}</Text>
              {record.mailingsConsent ? (
                <Badge color="grape" variant="light">
                  {t(($) => $.console.signupItems.mailingsBadge)}
                </Badge>
              ) : null}
            </Group>
          ),
        },
        {
          accessor: 'currentVersion',
          title: t(($) => $.console.signupItems.columns.version),
          render: (record) =>
            record.currentVersion ? (
              <Text size="sm">{record.currentVersion.label}</Text>
            ) : (
              <Badge color="gray" variant="light">
                {t(($) => $.console.signupItems.notPublished)}
              </Badge>
            ),
        },
      ]}
      onRowClick={({ record }) =>
        void navigate({ to: consoleSignupItemRoute.to, params: { itemId: record.id } })
      }
      emptyState={<EmptyState title={t(($) => $.console.signupItems.empty)} />}
    />
  )
}

// The Instance's Sign-up items (ADR 0062): a row opens the item and its Legal document. The form
// that adds one sits outside the list's boundary, so it stays usable when the list fails to load.
export function ConsoleSignupItemsPage() {
  const { t } = useTranslation()

  return (
    <Stack>
      <Title order={2}>{t(($) => $.console.signupItems.title)}</Title>
      <Text c="dimmed">{t(($) => $.console.signupItems.description)}</Text>
      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.console.signupItems.loadError)}
      >
        <SignupItemsTable />
      </QueryBoundary>
      <AddItemForm />
    </Stack>
  )
}
