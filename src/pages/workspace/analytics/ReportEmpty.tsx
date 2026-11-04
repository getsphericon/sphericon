import { Center, EmptyState } from '@mantine/core'
import type { EmptyStateProps } from '@mantine/core'

interface ReportEmptyProps extends EmptyStateProps {
  // The height of the chart the placeholder stands in for, so the report keeps its layout whether
  // the chart has data or not. Without it the report is empty as a whole: the placeholder fills
  // the free height of the section (AnalyticsLayout) and sits in its middle.
  chartHeight?: number
}

// ReportEmpty is the empty state of an analytics report, centered in the space it takes.
export function ReportEmpty({ chartHeight, ...props }: ReportEmptyProps) {
  return (
    <Center h={chartHeight} flex={chartHeight === undefined ? 1 : undefined}>
      <EmptyState {...props} />
    </Center>
  )
}
