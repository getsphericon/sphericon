import { Alert, Stack } from '@mantine/core'
import type { ReactNode } from 'react'

import { getApiErrorMessage } from '../utils/apiErrors.ts'
import type { ApiErrorLike } from '../utils/apiErrors.ts'
import { ProblemDebugPanel } from './ProblemDebugPanel.tsx'

interface ApiErrorAlertProps {
  error: ApiErrorLike | null | undefined
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
