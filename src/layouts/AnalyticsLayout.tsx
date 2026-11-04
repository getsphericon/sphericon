import { SegmentedControl, Stack } from '@mantine/core'
import { Outlet, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { zSiteAnalyticsRange } from '../generated/site/zod.gen.ts'
import { useAnalyticsRange } from '../hooks/useAnalyticsRange.ts'

const RANGES = zSiteAnalyticsRange.options

// AnalyticsLayout is the analytics section's shell: the one window every report shares (kept in
// the URL) and the report itself. The reports are listed in the workspace sidebar (AppNavbar).
export function AnalyticsLayout() {
  const { t } = useTranslation()
  const range = useAnalyticsRange()
  const navigate = useNavigate()

  // The section takes the main area's free height (the viewport less the header and the shell's
  // padding), so a report that is empty as a whole centers its ReportEmpty in what is left.
  return (
    <Stack mih="calc(100dvh - var(--app-shell-header-offset, 0rem) - 2 * var(--app-shell-padding))">
      <SegmentedControl
        w={{ base: '100%', sm: 'fit-content' }}
        value={range}
        onChange={(next) =>
          // The report being viewed stays. Resolved against the section itself, the target would
          // be its index, which redirects to the first report.
          void navigate({
            to: '.',
            search: (prev: Record<string, unknown>) => ({ ...prev, range: next }),
          })
        }
        data={RANGES.map((r) => ({ value: r, label: t(($) => $.analytics.range[r]) }))}
      />
      <Outlet />
    </Stack>
  )
}
