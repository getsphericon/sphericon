import { Skeleton, Stack } from '@mantine/core'

// The loading state of a whole page: a title bar over a few content blocks.
export function PageSkeleton() {
  return (
    <Stack>
      <Skeleton height={32} width="40%" />
      <Skeleton height={120} />
      <Skeleton height={120} />
    </Stack>
  )
}

// The loading state of a section or card: a few text lines.
export function BlockSkeleton({ lines = 3 }: { lines?: number }) {
  return (
    <Stack gap="xs">
      {Array.from({ length: lines }, (_, i) => (
        <Skeleton key={i} height={16} width={i === lines - 1 ? '60%' : '100%'} />
      ))}
    </Stack>
  )
}

// The loading state of a table: one bar per row, under the header the table draws itself.
export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <Stack gap="xs">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} height={36} />
      ))}
    </Stack>
  )
}
