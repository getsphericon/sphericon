import { Flex, SegmentedControl, Stack, Text } from '@mantine/core'
import { Outlet, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { NavLinkLink } from '../components/RouterLink.tsx'
import { zSiteAnalyticsRange } from '../generated/site/zod.gen.ts'
import { useAnalyticsRange } from '../hooks/useAnalyticsRange.ts'
import { Route as clicksByLinkRoute } from '../routes/workspaces/$workspaceId/analytics/clicks-by-link.tsx'
import { Route as emailMetricsRoute } from '../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { Route as eventsRoute } from '../routes/workspaces/$workspaceId/analytics/events.tsx'
import { Route as newPeopleRoute } from '../routes/workspaces/$workspaceId/analytics/new-people.tsx'
import { Route as opensByHourRoute } from '../routes/workspaces/$workspaceId/analytics/opens-by-hour.tsx'
import { Route as recentActivityRoute } from '../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics/route.tsx'
import { Route as subscribedPeopleRoute } from '../routes/workspaces/$workspaceId/analytics/subscribed-people.tsx'

const RANGES = zSiteAnalyticsRange.options

// AnalyticsLayout is the analytics section's shell: the secondary menu, the one window every
// report shares (kept in the URL) and the report itself. A new report adds one item to its group
// in `groups` below, plus its route and page.
export function AnalyticsLayout() {
  const { t } = useTranslation()
  const { workspaceId } = analyticsRoute.useParams()
  const range = useAnalyticsRange()
  const navigate = useNavigate()

  const groups = [
    {
      label: t(($) => $.analytics.nav.audience),
      items: [
        { route: newPeopleRoute, label: t(($) => $.analytics.nav.newPeople) },
        { route: subscribedPeopleRoute, label: t(($) => $.analytics.nav.subscribedPeople) },
      ],
    },
    {
      label: t(($) => $.analytics.nav.email),
      items: [
        { route: emailMetricsRoute, label: t(($) => $.analytics.nav.emailMetrics) },
        { route: clicksByLinkRoute, label: t(($) => $.analytics.nav.clicksByLink) },
        { route: opensByHourRoute, label: t(($) => $.analytics.nav.opensByHour) },
      ],
    },
    {
      label: t(($) => $.analytics.nav.tracking),
      items: [
        { route: eventsRoute, label: t(($) => $.analytics.nav.events) },
        { route: recentActivityRoute, label: t(($) => $.analytics.nav.recentActivity) },
      ],
    },
  ]

  return (
    <Flex
      direction={{ base: 'column', sm: 'row' }}
      align={{ base: 'stretch', sm: 'flex-start' }}
      gap="xl"
    >
      <Stack gap="md" miw={{ sm: 200 }}>
        {groups.map((group) => (
          <Stack key={group.label} gap={2}>
            <Text size="xs" fw={600} c="dimmed" tt="uppercase">
              {group.label}
            </Text>
            {group.items.map((item) => (
              <NavLinkLink
                key={item.route.to}
                to={item.route.to}
                params={{ workspaceId }}
                search={{ range }}
                label={item.label}
              />
            ))}
          </Stack>
        ))}
      </Stack>
      <Stack flex={1} miw={0}>
        <SegmentedControl
          w={{ base: '100%', sm: 'fit-content' }}
          value={range}
          onChange={(next) =>
            void navigate({
              from: analyticsRoute.fullPath,
              search: (prev) => ({ ...prev, range: next }),
            })
          }
          data={RANGES.map((r) => ({ value: r, label: t(($) => $.analytics.range[r]) }))}
        />
        <Outlet />
      </Stack>
    </Flex>
  )
}
