import { Button, Stack } from '@mantine/core'
import { QueryErrorResetBoundary, useQueryErrorResetBoundary } from '@tanstack/react-query'
import { Suspense, useState } from 'react'
import type { ReactElement, ReactNode } from 'react'
import { ErrorBoundary, useErrorBoundary } from 'react-error-boundary'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from './ApiErrorAlert.tsx'

interface FailedLoadProps {
  title: string
  // What a failed load shows instead of the alert, such as nothing for a section the caller may
  // not see. Undefined keeps the alert.
  renderError?: ((error: unknown) => ReactElement | null | undefined) | undefined
}

// FailedLoad is what the boundary shows in place of its content once a load failed: the alert
// with the API's detail, and a retry that fetches the failed queries again.
function FailedLoad({ title, renderError }: FailedLoadProps) {
  const { t } = useTranslation()
  const { error, resetBoundary } = useErrorBoundary()
  const custom = renderError?.(error)
  if (custom !== undefined) {
    return custom
  }
  return (
    <Stack gap="xs" align="flex-start">
      <ApiErrorAlert error={error} title={title} />
      <Button variant="default" size="xs" onClick={resetBoundary}>
        {t(($) => $.actions.retry)}
      </Button>
    </Stack>
  )
}

interface QueryBoundaryProps {
  // What shows while a suspense query below loads: a page, block or table skeleton.
  fallback: ReactNode
  // The title of the alert a failed load shows, with the API's detail under it.
  errorTitle: string
  renderError?: FailedLoadProps['renderError']
  children: ReactNode
}

function Boundary({ fallback, errorTitle, renderError, children }: QueryBoundaryProps) {
  const { reset } = useQueryErrorResetBoundary()
  // A suspense query whose failure is still cached throws it again without fetching unless its
  // reset boundary was reset. A boundary that mounts (the page is opened again) starts reset, so
  // an earlier failure is fetched again rather than shown again, as a plain query would.
  useState(reset)
  return (
    <ErrorBoundary
      onReset={reset}
      fallback={<FailedLoad title={errorTitle} renderError={renderError} />}
    >
      <Suspense fallback={fallback}>{children}</Suspense>
    </ErrorBoundary>
  )
}

// QueryBoundary owns the load states of the suspense queries below it: the fallback while they
// load and the error alert when one fails, so the component that reads them renders data only.
export function QueryBoundary(props: QueryBoundaryProps) {
  return (
    <QueryErrorResetBoundary>
      <Boundary {...props} />
    </QueryErrorResetBoundary>
  )
}
