import { useQueryClient } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { useApiErrorMessage } from '../hooks/useApiErrorMessage.ts'
import { useNotify } from '../hooks/useNotify.ts'
import { invalidateOwn } from '../hooks/useResourceMutation.ts'
import type { EditorEffects } from './editorSession.ts'

// The toasts of an operation's outcome. `failedMessage` stands in when the API gives no detail.
export interface EditorOutcomeTexts {
  accepted: string
  failedTitle: string
  failedMessage?: string
}

// The effects of an editor session over a resource cached under `queryKey`, its get query. The
// write's response is authoritative: an older GET is cancelled before the response is cached, and
// the queries of the resource's own tags (its list, its detail) refresh in the background.
export function useEditorEffects<TResource, TOperation extends string>(
  queryKey: QueryKey,
  texts: (operation: TOperation | 'SAVE') => EditorOutcomeTexts,
): EditorEffects<TResource, TOperation> {
  const { t } = useTranslation()
  const notify = useNotify()
  const apiErrorMessage = useApiErrorMessage()
  const queryClient = useQueryClient()

  return {
    store: (resource) => {
      void queryClient.cancelQueries({ queryKey })
      queryClient.setQueryData<TResource>(queryKey, resource)
    },
    invalidate: () => {
      void invalidateOwn(queryClient, queryKey)
    },
    accepted: (operation) => {
      notify({
        color: 'teal',
        title: t(($) => $.notifications.successTitle),
        message: texts(operation).accepted,
      })
    },
    failed: (operation, error) => {
      const { failedTitle, failedMessage } = texts(operation)
      notify({
        color: 'red',
        title: failedTitle,
        message: apiErrorMessage(error, failedMessage ?? t(($) => $.notifications.errorMessage)),
      })
    },
  }
}
