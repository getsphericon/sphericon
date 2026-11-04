import { Card, Flex, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconAlertTriangle, IconCircleX, IconInfoCircle } from '@tabler/icons-react'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { Fragment } from 'react'
import type { ComponentType } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { siteAttentionGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteAttentionItem,
  SiteAttentionKind,
  SiteAttentionSeverity,
} from '../../generated/site/types.gen.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import { Route as automationRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as broadcastReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as emailsRoute } from '../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { Route as webhooksRoute } from '../../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { Route as suppressionsRoute } from '../../routes/workspaces/$workspaceId/suppressions/index.tsx'
import { holdExplanation } from '../../utils/hold.ts'

const SEVERITY_ICON = {
  blocked: { color: 'red', Icon: IconCircleX },
  degraded: { color: 'yellow', Icon: IconAlertTriangle },
  notice: { color: 'gray', Icon: IconInfoCircle },
} as const satisfies Record<SiteAttentionSeverity, unknown>

const nameOf = (item: SiteAttentionItem) => item.subject?.name ?? ''

type Format = ReturnType<typeof useFormat>

// MESSAGE is each kind of item as a sentence about its object; an instant it names is shown in the
// User time zone.
const MESSAGE = {
  sendingDomainVerificationLost: (t, item) =>
    t(($) => $.overview.attention.kinds.sendingDomainVerificationLost, { name: nameOf(item) }),
  integrationProviderStopped: (t, item) =>
    t(($) => $.overview.attention.kinds.integrationProviderStopped, { name: nameOf(item) }),
  integrationProviderRefused: (t, item) =>
    t(($) => $.overview.attention.kinds.integrationProviderRefused, { name: nameOf(item) }),
  broadcastHeld: (t, item) =>
    t(($) => $.overview.attention.kinds.broadcastHeld, {
      name: nameOf(item),
      reason: item.holdReason ? holdExplanation(t, item.holdReason) : '',
    }),
  automationCannotSend: (t, item) =>
    t(($) => $.overview.attention.kinds.automationCannotSend, {
      name: nameOf(item),
      reason: item.holdReason ? holdExplanation(t, item.holdReason) : '',
    }),
  deliverabilityWarning: (t, item, format) => {
    const metric = item.abuse?.metric ?? 'bounce'
    return t(($) => $.overview.attention.kinds.deliverabilityWarning, {
      metric: t(($) => $.overview.attention.metrics[metric]),
      rate: format.formatPercent(item.abuse?.rate ?? 0),
      domain: item.abuse?.domain ?? '',
    })
  },
  sendCapSpent: (t) => t(($) => $.overview.attention.kinds.sendCapSpent),
  scheduledBroadcastOverSendCap: (t, item, format) =>
    t(($) => $.overview.attention.kinds.scheduledBroadcastOverSendCap, {
      name: nameOf(item),
      count: item.count,
      finish: item.estimatedCompletionAt ? format.formatDateTime(item.estimatedCompletionAt) : '',
    }),
  providerLimitSpent: (t, item) =>
    t(($) => $.overview.attention.kinds.providerLimitSpent, { name: nameOf(item) }),
  integrationProviderProbation: (t, item) =>
    t(($) => $.overview.attention.kinds.integrationProviderProbation, { name: nameOf(item) }),
  integrationProviderSandbox: (t, item) =>
    t(($) => $.overview.attention.kinds.integrationProviderSandbox, { name: nameOf(item) }),
  enrollmentsFailed: (t, item) =>
    t(($) => $.overview.attention.kinds.enrollmentsFailed, {
      name: nameOf(item),
      count: item.count,
    }),
  broadcastFailed: (t, item) =>
    t(($) => $.overview.attention.kinds.broadcastFailed, { name: nameOf(item) }),
  webhookDisabled: (t, item) =>
    t(($) => $.overview.attention.kinds.webhookDisabled, { name: nameOf(item) }),
  sendingDomainNotBulkReady: (t, item) =>
    t(($) => $.overview.attention.kinds.sendingDomainNotBulkReady, { name: nameOf(item) }),
} as const satisfies Record<
  SiteAttentionKind,
  (t: TFunction, item: SiteAttentionItem, format: Format) => string
>

// GROUP_MESSAGE is a kind whose items differ only by the object they are about, said once for
// several of them: the block names the objects below it instead of repeating the sentence. A held
// Broadcast and an Automation that cannot send are grouped by their reason, which the sentence
// carries. The kinds missing here carry figures of their own and are never grouped.
const GROUP_MESSAGE = {
  sendingDomainVerificationLost: (t, _item, count) =>
    t(($) => $.overview.attention.groups.sendingDomainVerificationLost, { count }),
  integrationProviderStopped: (t, _item, count) =>
    t(($) => $.overview.attention.groups.integrationProviderStopped, { count }),
  integrationProviderRefused: (t, _item, count) =>
    t(($) => $.overview.attention.groups.integrationProviderRefused, { count }),
  broadcastHeld: (t, item, count) =>
    t(($) => $.overview.attention.groups.broadcastHeld, {
      count,
      reason: item.holdReason ? holdExplanation(t, item.holdReason) : '',
    }),
  automationCannotSend: (t, item, count) =>
    t(($) => $.overview.attention.groups.automationCannotSend, {
      count,
      reason: item.holdReason ? holdExplanation(t, item.holdReason) : '',
    }),
  providerLimitSpent: (t, _item, count) =>
    t(($) => $.overview.attention.groups.providerLimitSpent, { count }),
  integrationProviderProbation: (t, _item, count) =>
    t(($) => $.overview.attention.groups.integrationProviderProbation, { count }),
  integrationProviderSandbox: (t, _item, count) =>
    t(($) => $.overview.attention.groups.integrationProviderSandbox, { count }),
  broadcastFailed: (t, _item, count) =>
    t(($) => $.overview.attention.groups.broadcastFailed, { count }),
  webhookDisabled: (t, _item, count) =>
    t(($) => $.overview.attention.groups.webhookDisabled, { count }),
  sendingDomainNotBulkReady: (t, _item, count) =>
    t(($) => $.overview.attention.groups.sendingDomainNotBulkReady, { count }),
} as const satisfies Partial<
  Record<SiteAttentionKind, (t: TFunction, item: SiteAttentionItem, count: number) => string>
>

const isGrouped = (kind: SiteAttentionKind): kind is keyof typeof GROUP_MESSAGE =>
  Object.hasOwn(GROUP_MESSAGE, kind)

// groupKey is the row an item belongs to: one per grouped kind (and hold reason), one per item of
// any other kind.
const groupKey = (item: SiteAttentionItem) =>
  isGrouped(item.kind)
    ? `${item.kind}:${item.holdReason ?? ''}`
    : `${item.kind}:${item.subject?.id ?? ''}`

type Row = [SiteAttentionItem, ...SiteAttentionItem[]]

// rowsOf groups the items into rows. A row stands where its first item stood, so the most urgent
// and the oldest still come first.
function rowsOf(items: SiteAttentionItem[]): Row[] {
  const rows = new Map<string, Row>()
  for (const item of items) {
    const key = groupKey(item)
    const row = rows.get(key)
    if (row === undefined) {
      rows.set(key, [item])
    } else {
      row.push(item)
    }
  }
  return [...rows.values()]
}

interface ActionProps {
  workspaceId: string
  id: string
  // label is the link's text in place of the action's own.
  label?: string
}

function EmailsAction({ workspaceId }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={emailsRoute.to} params={{ workspaceId }} size="sm">
      {t(($) => $.overview.attention.actions.emails)}
    </AnchorLink>
  )
}

function BroadcastAction({ workspaceId, id, label }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={broadcastReportRoute.to} params={{ workspaceId, broadcastId: id }} size="sm">
      {label ?? t(($) => $.overview.attention.actions.broadcast)}
    </AnchorLink>
  )
}

function AutomationAction({ workspaceId, id, label }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={automationRoute.to} params={{ workspaceId, automationId: id }} size="sm">
      {label ?? t(($) => $.overview.attention.actions.automation)}
    </AnchorLink>
  )
}

function WebhooksAction({ workspaceId }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={webhooksRoute.to} params={{ workspaceId }} size="sm">
      {t(($) => $.overview.attention.actions.webhooks)}
    </AnchorLink>
  )
}

function SuppressionsAction({ workspaceId }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={suppressionsRoute.to} params={{ workspaceId }} size="sm">
      {t(($) => $.overview.attention.actions.suppressions)}
    </AnchorLink>
  )
}

// ACTION is where each kind of item is fixed; the Send cap refills by itself and has none.
const ACTION = {
  sendingDomainVerificationLost: EmailsAction,
  integrationProviderStopped: EmailsAction,
  integrationProviderRefused: EmailsAction,
  broadcastHeld: BroadcastAction,
  automationCannotSend: AutomationAction,
  deliverabilityWarning: SuppressionsAction,
  sendCapSpent: null,
  scheduledBroadcastOverSendCap: BroadcastAction,
  providerLimitSpent: EmailsAction,
  integrationProviderProbation: EmailsAction,
  integrationProviderSandbox: EmailsAction,
  enrollmentsFailed: AutomationAction,
  broadcastFailed: BroadcastAction,
  webhookDisabled: WebhooksAction,
  sendingDomainNotBulkReady: EmailsAction,
} as const satisfies Record<SiteAttentionKind, ComponentType<ActionProps> | null>

// OBJECT_ACTIONS open the object an item is about rather than a page of the Workspace: in a group,
// each name links to its object and the row has no link of its own.
const OBJECT_ACTIONS: ReadonlySet<ComponentType<ActionProps>> = new Set([
  BroadcastAction,
  AutomationAction,
])

// Names are the objects of a grouped row, each a link to it where it has a page of its own.
function Names({ items, workspaceId }: { items: Row; workspaceId: string }) {
  const Action = ACTION[items[0].kind]
  const links = Action !== null && OBJECT_ACTIONS.has(Action)
  return (
    <Text size="sm" c="dimmed">
      {items.map((item, index) => (
        <Fragment key={item.subject?.id ?? index}>
          {index > 0 && ', '}
          {links ? (
            <Action workspaceId={workspaceId} id={item.subject?.id ?? ''} label={nameOf(item)} />
          ) : (
            nameOf(item)
          )}
        </Fragment>
      ))}
    </Text>
  )
}

// AttentionRow is one item, or several of a grouped kind said once with their names below. The
// link to where it is fixed sits beside the sentence on a wide screen and under it on a phone.
function AttentionRow({ items, workspaceId }: { items: Row; workspaceId: string }) {
  const { t } = useTranslation()
  const format = useFormat()
  const [first] = items
  // Only a grouped kind's row holds more than one item.
  const grouped = items.length > 1
  const { color, Icon } = SEVERITY_ICON[first.severity]
  const Action = ACTION[first.kind]
  const showAction = Action !== null && !(grouped && OBJECT_ACTIONS.has(Action))
  const message =
    grouped && isGrouped(first.kind)
      ? GROUP_MESSAGE[first.kind](t, first, items.length)
      : MESSAGE[first.kind](t, first, format)

  return (
    <Group gap="sm" wrap="nowrap" align="flex-start">
      <ThemeIcon color={color} variant="light" radius="xl" size="md">
        <Icon size={16} />
      </ThemeIcon>
      <Stack gap={2} flex={1}>
        <Flex
          direction={{ base: 'column', sm: 'row' }}
          justify="space-between"
          align={{ base: 'flex-start', sm: 'baseline' }}
          gap={{ base: 2, sm: 'md' }}
        >
          <Text size="sm">{message}</Text>
          {showAction && (
            <Text size="sm" flex="none">
              <Action workspaceId={workspaceId} id={first.subject?.id ?? ''} />
            </Text>
          )}
        </Flex>
        {grouped && <Names items={items} workspaceId={workspaceId} />}
      </Stack>
    </Group>
  )
}

// AttentionItems is what needs a member's action now, most urgent first, and nothing at all while
// nothing does: the block is never an "all good" box.
function AttentionItems({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const items = useSuspenseQuery(siteAttentionGetOptions({ path: { workspaceId } })).data.items
  if (items.length === 0) {
    return null
  }

  return (
    <Card withBorder padding="lg">
      <Stack gap="sm">
        <Title order={4}>{t(($) => $.overview.attention.title)}</Title>
        {rowsOf(items).map((row) => (
          <AttentionRow key={groupKey(row[0])} items={row} workspaceId={workspaceId} />
        ))}
      </Stack>
    </Card>
  )
}

// AttentionBlock is the overview's first block. It shows no skeleton while it loads: most of the
// time nothing needs attention and the block is not there.
export function AttentionBlock({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  return (
    <QueryBoundary fallback={null} errorTitle={t(($) => $.overview.attention.loadError)}>
      <AttentionItems workspaceId={workspaceId} />
    </QueryBoundary>
  )
}
