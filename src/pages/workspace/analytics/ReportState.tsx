import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../../components/ApiErrorAlert.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import type { ApiErrorLike } from '../../../utils/apiErrors.ts'

interface ReportStateProps {
  query: { isLoading: boolean; isError: boolean; error: ApiErrorLike | null | undefined }
  // The failure title; every report shares one unless it names its own.
  errorMessage?: string
}

// ReportState is what an analytics report shows while its query loads or after it failed, and
// nothing otherwise: the page renders its data next to it.
export function ReportState({ query, errorMessage }: ReportStateProps) {
  const { t } = useTranslation()
  if (query.isLoading) {
    return <BlockSkeleton />
  }
  if (query.isError) {
    const message = errorMessage ?? t(($) => $.analytics.loadError)
    return <ApiErrorAlert error={query.error} title={message} fallback={message} />
  }
  return null
}
