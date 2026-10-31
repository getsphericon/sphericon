import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'

interface ReportBoundaryProps {
  // The failure title; every report shares one unless it names its own.
  errorTitle?: string
  children: ReactNode
}

// ReportBoundary owns the load states of an analytics report: a block skeleton while its query
// loads and the failure alert after. It starts over when the section's window changes, so a window
// that failed does not stay on screen.
export function ReportBoundary({ errorTitle, children }: ReportBoundaryProps) {
  const { t } = useTranslation()
  const range = useAnalyticsRange()
  return (
    <QueryBoundary
      key={range}
      fallback={<BlockSkeleton />}
      errorTitle={errorTitle ?? t(($) => $.analytics.loadError)}
    >
      {children}
    </QueryBoundary>
  )
}
