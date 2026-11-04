import { Center, EmptyState } from '@mantine/core'
import type { EmptyStateProps } from '@mantine/core'

interface CenteredEmptyStateProps extends EmptyStateProps {
  // The height of the chart the placeholder stands in for, so a report keeps its layout whether
  // the chart has data or not. Without it the placeholder grows into the free height of its flex
  // column (the page, which DashboardShell stretches to the main area) and sits in its middle.
  height?: number
}

// CenteredEmptyState is the empty state of a list or a report, centered in the space it takes
// (docs/standards/ui.md, "Empty states").
export function CenteredEmptyState({ height, ...props }: CenteredEmptyStateProps) {
  return (
    <Center h={height} flex={height === undefined ? 1 : undefined}>
      <EmptyState {...props} />
    </Center>
  )
}
