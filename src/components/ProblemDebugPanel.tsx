import { Code, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { getProblemDebug } from '../utils/apiErrors.ts'

interface ProblemDebugPanelProps {
  error: unknown
}

// ProblemDebugPanel shows the fault a development server attaches to a 500, the way Rails'
// error page does: the error, the request and the stack of a panic. It renders nothing for an
// error without that block, which is every error a production Instance answers.
export function ProblemDebugPanel({ error }: ProblemDebugPanelProps) {
  const { t } = useTranslation()
  const debug = getProblemDebug(error)

  if (debug === undefined) {
    return null
  }

  return (
    <Stack gap="xs" ta="left" w="100%">
      <Code block c="red">
        {debug.error}
      </Code>
      <Text size="sm" c="dimmed">
        {t(($) => $.appError.debugRequest, { method: debug.method, path: debug.path })}
        {debug.requestId === undefined
          ? ''
          : ` · ${t(($) => $.appError.debugRequestId, { id: debug.requestId })}`}
      </Text>
      {debug.stack === undefined ? null : <Code block>{debug.stack}</Code>}
    </Stack>
  )
}
