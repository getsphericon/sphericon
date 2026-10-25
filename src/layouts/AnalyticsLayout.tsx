import { SegmentedControl, Stack } from '@mantine/core'
import { Outlet, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { zSiteAnalyticsRange } from '../generated/site/zod.gen.ts'
import { useAnalyticsRange } from '../hooks/useAnalyticsRange.ts'
import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics/route.tsx'

const RANGES = zSiteAnalyticsRange.options

// AnalyticsLayout is the analytics section's shell: the one window every report shares (kept in
// the URL) and the report itself. The reports are listed in the workspace sidebar (AppNavbar).
export function AnalyticsLayout() {
  const { t } = useTranslation()
  const range = useAnalyticsRange()
  const navigate = useNavigate()

  return (
    <Stack>
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
  )
}
