import { Alert } from '@mantine/core'
import type { ReactNode } from 'react'

import { getApiErrorMessage } from '../utils/apiErrors.ts'
import type { ApiErrorLike } from '../utils/apiErrors.ts'

interface ApiErrorAlertProps {
  error: ApiErrorLike | null | undefined
  title: ReactNode
  fallback: string
}

export function ApiErrorAlert({ error, title, fallback }: ApiErrorAlertProps) {
  return (
    <Alert color="red" title={title}>
      {getApiErrorMessage(error, fallback)}
    </Alert>
  )
}
