import { Group, SegmentedControl, Stack, Text } from '@mantine/core'
import { Outlet, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { NavLinkLink } from '../components/RouterLink.tsx'
import { zSiteAnalyticsRange } from '../generated/site/zod.gen.ts'
import { useAnalyticsRange } from '../hooks/useAnalyticsRange.ts'
import { Route as emailMetricsRoute } from '../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { Route as recentActivityRoute } from '../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics/route.tsx'

const RANGES = zSiteAnalyticsRange.options

// AnalyticsLayout is the analytics section's shell: the secondary menu, the one window every
// report shares (kept in the URL) and the report itself. A new report adds one item to its group
// in `groups` below, plus its route and page; a group with no report yet is not shown.
export function AnalyticsLayout() {
  const { t } = useTranslation()
  const { workspaceId } = analyticsRoute.useParams()
  const range = useAnalyticsRange()
  const navigate = useNavigate()

  const groups = [
    { label: t(($) => $.analytics.nav.audience), items: [] },
    {
      label: t(($) => $.analytics.nav.email),
      items: [{ route: emailMetricsRoute, label: t(($) => $.analytics.nav.emailMetrics) }],
    },
    {
      label: t(($) => $.analytics.nav.tracking),
      items: [{ route: recentActivityRoute, label: t(($) => $.analytics.nav.recentActivity) }],
    },
  ]

  return (
    <Group align="flex-start" wrap="nowrap" gap="xl">
      <Stack gap="md" miw={200}>
        {groups
          .filter((group) => group.items.length > 0)
          .map((group) => (
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
          w="fit-content"
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
    </Group>
  )
}
