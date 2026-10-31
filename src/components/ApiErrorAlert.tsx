import { Alert, Stack } from '@mantine/core'
import type { ReactNode } from 'react'

import { getApiErrorMessage } from '../utils/apiErrors.ts'
import { ProblemDebugPanel } from './ProblemDebugPanel.tsx'

interface ApiErrorAlertProps {
  error: unknown
  title: ReactNode
  fallback: string
}

export function ApiErrorAlert({ error, title, fallback }: ApiErrorAlertProps) {
  return (
    <Alert color="red" title={title}>
      <Stack gap="xs">
        {getApiErrorMessage(error, fallback)}
        <ProblemDebugPanel error={error} />
      </Stack>
    </Alert>
  )
}
