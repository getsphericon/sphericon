import { Alert, Stack } from '@mantine/core'
import type { ReactNode } from 'react'

import { getProblemDebug, getProblemMessage } from '../utils/apiErrors.ts'
import { ProblemDebugPanel } from './ProblemDebugPanel.tsx'

interface ApiErrorAlertProps {
  error: unknown
  title: ReactNode
  // What the body says when the error carries no message of its own. Without it the title
  // alone tells what failed.
  fallback?: string
}

// ApiErrorAlert shows a failed call: the title, the API's own message under it (or the
// fallback), and the fault a development server attaches.
export function ApiErrorAlert({ error, title, fallback }: ApiErrorAlertProps) {
  const message = getProblemMessage(error) ?? fallback
  if (message === undefined && getProblemDebug(error) === undefined) {
    return <Alert color="red" title={title} />
  }
  return (
    <Alert color="red" title={title}>
      <Stack gap="xs">
        {message}
        <ProblemDebugPanel error={error} />
      </Stack>
    </Alert>
  )
}
