import { Button, Group, Stack, Text, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { siteWorkspacesGetSendingRulesOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import type {
  FrequencyCap,
  FrequencyCapChannel,
  SendingWindow,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { useWeekdayNames } from '../../../hooks/useWeekdayNames.ts'
import { can } from '../../../utils/permissions.ts'
import { useChannelNames } from './sendingRuleNames.ts'
import { clock, periodIn } from './sendingRules.ts'
import { sendingRulesEditor } from './workspaceSettingsEditors.ts'

// Where a rule applies: the Channel, or the Channel in one country.
function useRuleScope() {
  const { t } = useTranslation()
  const channels = useChannelNames()
  return (channel: FrequencyCapChannel, country: string | null) =>
    country === null
      ? channels[channel]
      : t(($) => $.settings.sendingRules.inCountry, { channel: channels[channel], country })
}

function CapLine({ cap }: { cap: FrequencyCap }) {
  const { t } = useTranslation()
  const scope = useRuleScope()
  if (cap.maxMessages === null || cap.periodMinutes === null) {
    return (
      <Text size="sm">
        {t(($) => $.settings.sendingRules.noCap, { scope: scope(cap.channel, cap.country) })}
      </Text>
    )
  }
  const period = periodIn(cap.periodMinutes)
  const periods = {
    days: t(($) => $.settings.sendingRules.periodDays, { count: period.count }),
    hours: t(($) => $.settings.sendingRules.periodHours, { count: period.count }),
    minutes: t(($) => $.settings.sendingRules.periodMinutes, { count: period.count }),
  }
  return (
    <Text size="sm">
      {t(($) => $.settings.sendingRules.capLine, {
        scope: scope(cap.channel, cap.country),
        max: cap.maxMessages,
        period: periods[period.unit],
      })}
    </Text>
  )
}

function WindowLine({ window }: { window: SendingWindow }) {
  const { t } = useTranslation()
  const scope = useRuleScope()
  const days = useWeekdayNames()
  const hours = window.hours
    .map((h) =>
      t(($) => $.settings.sendingRules.dayHours, {
        day: days[h.day],
        opens: clock(h.opensAt),
        closes: clock(h.closesAt),
      }),
    )
    .join(', ')
  return (
    <Text size="sm">
      {t(($) => $.settings.sendingRules.windowLine, {
        scope: scope(window.channel, window.country),
        hours,
      })}
    </Text>
  )
}

function SendingRules({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const [opened, { open, close }] = useDisclosure(false)
  const { data } = useSuspenseQuery(
    siteWorkspacesGetSendingRulesOptions({ path: { workspaceId: workspace.id } }),
  )
  return (
    <Stack gap="xs">
      <Text size="sm" fw={500}>
        {t(($) => $.settings.sendingRules.capsTitle)}
      </Text>
      {data.frequencyCaps.length === 0 ? (
        <Text size="sm">{t(($) => $.settings.sendingRules.noCaps)}</Text>
      ) : (
        data.frequencyCaps.map((cap) => <CapLine key={`${cap.channel}-${cap.country}`} cap={cap} />)
      )}
      <Text size="sm" fw={500}>
        {t(($) => $.settings.sendingRules.windowsTitle)}
      </Text>
      {data.sendingWindows.length === 0 ? (
        <Text size="sm">{t(($) => $.settings.sendingRules.noWindows)}</Text>
      ) : (
        data.sendingWindows.map((window) => (
          <WindowLine key={`${window.channel}-${window.country}`} window={window} />
        ))
      )}
      {/* Only an Owner or Admin changes them (ADR 0137); every member reads them. */}
      {can(workspace, 'workspace:update') ? (
        <Group>
          <Button variant="light" onClick={open}>
            {t(($) => $.settings.sendingRules.change)}
          </Button>
        </Group>
      ) : null}
      <sendingRulesEditor.EditModal
        id={opened ? 'sendingRules' : null}
        onClose={close}
        title={t(($) => $.settings.sendingRules.formTitle)}
        size="xl"
      />
    </Stack>
  )
}

// SendingRulesSection holds the Workspace's Frequency caps and Sending windows (ADR 0134, ADR 0137)
// as text, with a Change action that opens their form. It says that the Workspace answers for its
// compliance: the defaults are not legal advice.
export function SendingRulesSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.sendingRules.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.sendingRules.compliance)}
      </Text>
      <QueryBoundary
        fallback={<BlockSkeleton />}
        errorTitle={t(($) => $.settings.sendingRules.loadError)}
      >
        <SendingRules workspace={workspace} />
      </QueryBoundary>
    </Stack>
  )
}
