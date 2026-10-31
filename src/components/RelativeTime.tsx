import { Text, Tooltip } from '@mantine/core'

import { useDateFormat } from '../hooks/useDateFormat.ts'
import { formatRelative } from '../utils/datetime.ts'

// RelativeTime shows a moment as "5 minutes ago", with the exact date and time on hover. For
// feeds, where how recent a row is matters more than its timestamp. The exact time is in the User
// time zone (ADR 0068).
export function RelativeTime({ value }: { value: string }) {
  const { formatDateTime } = useDateFormat()
  return (
    <Tooltip label={formatDateTime(value)} withArrow>
      <Text span inherit>
        <time dateTime={value}>{formatRelative(value)}</time>
      </Text>
    </Tooltip>
  )
}
