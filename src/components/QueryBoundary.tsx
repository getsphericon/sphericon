import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { Suspense } from 'react'
import type { ReactElement, ReactNode } from 'react'
import { ErrorBoundary, useErrorBoundary } from 'react-error-boundary'

import { ApiErrorAlert } from './ApiErrorAlert.tsx'

interface FailedLoadProps {
  title: string
  // What a failed load shows instead of the alert, such as nothing for a section the caller may
  // not see. Undefined keeps the alert.
  renderError?: ((error: unknown) => ReactElement | null | undefined) | undefined
}

// FailedLoad is what the boundary shows in place of its content once a load failed.
function FailedLoad({ title, renderError }: FailedLoadProps) {
  const { error } = useErrorBoundary()
  const custom = renderError?.(error)
  return custom === undefined ? (
    <ApiErrorAlert error={error} title={title} fallback={title} />
  ) : (
    custom
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

// QueryBoundary owns the load states of the suspense queries below it: the fallback while they
// load and the error alert when one fails, so the component that reads them renders data only.
// Resetting the boundary lets the failed queries fetch again.
export function QueryBoundary({ fallback, errorTitle, renderError, children }: QueryBoundaryProps) {
  const { reset } = useQueryErrorResetBoundary()
  return (
    <ErrorBoundary
      onReset={reset}
      fallback={<FailedLoad title={errorTitle} renderError={renderError} />}
    >
      <Suspense fallback={fallback}>{children}</Suspense>
    </ErrorBoundary>
  )
}
