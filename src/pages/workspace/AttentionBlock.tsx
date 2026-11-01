import { Card, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconAlertTriangle, IconCircleX, IconInfoCircle } from '@tabler/icons-react'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
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
import { Route as automationRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as broadcastReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as emailsRoute } from '../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { Route as webhooksRoute } from '../../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { Route as suppressionsRoute } from '../../routes/workspaces/$workspaceId/suppressions/index.tsx'
import { holdExplanation } from '../../utils/broadcastHold.ts'
import { formatPercent } from '../../utils/percent.ts'

const SEVERITY_ICON = {
  blocked: { color: 'red', Icon: IconCircleX },
  degraded: { color: 'yellow', Icon: IconAlertTriangle },
  notice: { color: 'gray', Icon: IconInfoCircle },
} as const satisfies Record<SiteAttentionSeverity, unknown>

const nameOf = (item: SiteAttentionItem) => item.subject?.name ?? ''

// MESSAGE is each kind of item as a sentence about its object.
const MESSAGE = {
  sendingDomainVerificationLost: (t, item) =>
    t(($) => $.overview.attention.kinds.sendingDomainVerificationLost, { name: nameOf(item) }),
  integrationProviderStopped: (t, item) =>
    t(($) => $.overview.attention.kinds.integrationProviderStopped, { name: nameOf(item) }),
  broadcastHeld: (t, item) =>
    t(($) => $.overview.attention.kinds.broadcastHeld, {
      name: nameOf(item),
      reason: holdExplanation(t, item.holdReason ?? ''),
    }),
  deliverabilityWarning: (t, item) => {
    const metric = item.abuse?.metric ?? 'bounce'
    return t(($) => $.overview.attention.kinds.deliverabilityWarning, {
      metric: t(($) => $.overview.attention.metrics[metric]),
      rate: formatPercent(item.abuse?.rate ?? 0),
      domain: item.abuse?.domain ?? '',
    })
  },
  sendCapSpent: (t) => t(($) => $.overview.attention.kinds.sendCapSpent),
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
} as const satisfies Record<SiteAttentionKind, (t: TFunction, item: SiteAttentionItem) => string>

interface ActionProps {
  workspaceId: string
  id: string
}

function EmailsAction({ workspaceId }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={emailsRoute.to} params={{ workspaceId }} size="sm">
      {t(($) => $.overview.attention.actions.emails)}
    </AnchorLink>
  )
}

function BroadcastAction({ workspaceId, id }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={broadcastReportRoute.to} params={{ workspaceId, broadcastId: id }} size="sm">
      {t(($) => $.overview.attention.actions.broadcast)}
    </AnchorLink>
  )
}

function AutomationAction({ workspaceId, id }: ActionProps) {
  const { t } = useTranslation()
  return (
    <AnchorLink to={automationRoute.to} params={{ workspaceId, automationId: id }} size="sm">
      {t(($) => $.overview.attention.actions.automation)}
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
  broadcastHeld: BroadcastAction,
  deliverabilityWarning: SuppressionsAction,
  sendCapSpent: null,
  providerLimitSpent: EmailsAction,
  integrationProviderProbation: EmailsAction,
  integrationProviderSandbox: EmailsAction,
  enrollmentsFailed: AutomationAction,
  broadcastFailed: BroadcastAction,
  webhookDisabled: WebhooksAction,
} as const satisfies Record<SiteAttentionKind, ComponentType<ActionProps> | null>

// ItemAction is the link to where the item is fixed, when there is one.
function ItemAction({ item, workspaceId }: { item: SiteAttentionItem; workspaceId: string }) {
  const Action = ACTION[item.kind]
  return Action === null ? null : <Action workspaceId={workspaceId} id={item.subject?.id ?? ''} />
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
        {items.map((item) => {
          const { color, Icon } = SEVERITY_ICON[item.severity]
          return (
            <Group
              key={`${item.kind}-${item.subject?.id ?? ''}`}
              gap="sm"
              wrap="nowrap"
              align="flex-start"
            >
              <ThemeIcon color={color} variant="light" radius="xl" size="md">
                <Icon size={16} />
              </ThemeIcon>
              <Stack gap={2}>
                <Text size="sm">{MESSAGE[item.kind](t, item)}</Text>
                <ItemAction item={item} workspaceId={workspaceId} />
              </Stack>
            </Group>
          )
        })}
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
