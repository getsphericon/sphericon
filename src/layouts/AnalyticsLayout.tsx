import { SegmentedControl, Stack } from '@mantine/core'
import { Outlet, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import type { SiteAnalyticsRange } from '../generated/site/types.gen.ts'
import { zSiteAnalyticsRange } from '../generated/site/zod.gen.ts'
import { useAnalyticsRange } from '../hooks/useAnalyticsRange.ts'

const RANGES = zSiteAnalyticsRange.options

// AnalyticsLayout is the analytics section's shell: the one window every report shares (kept in
// the URL) and the report itself. The reports are listed in the workspace sidebar (AppNavbar).
export function AnalyticsLayout() {
  const { t } = useTranslation()
  const range = useAnalyticsRange()
  const navigate = useNavigate()

  // The section grows into the main area's free height, so a report that is empty as a whole
  // centers its CenteredEmptyState in what is left below the window.
  return (
    <Stack flex={1}>
      <SegmentedControl<SiteAnalyticsRange>
        w={{ base: '100%', sm: 'fit-content' }}
        value={range}
        onChange={(next) =>
          // The report being viewed stays. Resolved against the section itself, the target would
          // be its index, which redirects to the first report.
          void navigate({
            to: '.',
            search: (prev) => ({ ...prev, range: next }),
          })
        }
        data={RANGES.map((r) => ({ value: r, label: t(($) => $.analytics.range[r]) }))}
      />
      <Outlet />
    </Stack>
  )
}
