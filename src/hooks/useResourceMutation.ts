import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { QueryKey, UseMutationOptions } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { isForbiddenError } from '../utils/apiErrors.ts'
import { useApiErrorMessage } from './useApiErrorMessage.ts'
import { useNotify } from './useNotify.ts'

// Options layered on top of a generated mutation. The three things every CRUD
// mutation in this app repeats — invalidate the queries it affects, toast on
// success, toast the API error on failure — collapse into declarative fields.
interface ResourceMutationOptions<TData, TError, TVars, TCtx> {
  // The generated react-query mutation, e.g. siteTemplatesCreateMutation().
  mutation: UseMutationOptions<TData, TError, TVars, TCtx>
  // A successful mutation invalidates every query of its own resource: the generated keys carry
  // their operation's tags (`Site Segments`), so nothing is listed for that. List here only the
  // queries of another resource the mutation changes (a Brand kit save recreates Templates);
  // false skips the own-resource invalidation too, for a mutation that ends the session.
  invalidate?: QueryKey[] | false
  // Toast message shown on success. Omit for no toast (the generic "Success"
  // title is supplied by the hook).
  successMessage?: string
  // Title used for the error toast. The message is the API error detail, or a
  // generic fallback when the API gives none — never a copy of the title.
  errorTitle: string
  // Message shown instead of the API detail when the API answers 403 (the action
  // is role-gated and the caller lacks the role) — localized, unlike the detail.
  forbiddenMessage?: string
  // Takes over a failure before the toast: true when it dealt with the error itself (no toast).
  handleError?: (error: TError) => boolean | Promise<boolean>
  // Extra success work — navigation, form reset — run after invalidation.
  onDone?: (data: TData, variables: TVars) => void | Promise<void>
}

// useResourceMutation wraps a generated mutation with the app's standard
// success/error UX so call sites stay declarative. It keeps react-query's typing
// by inferring TData/TError/TVars from the passed mutation options.
export function useResourceMutation<TData, TError, TVars, TCtx>({
  mutation,
  invalidate,
  successMessage,
  errorTitle,
  forbiddenMessage,
  handleError,
  onDone,
}: ResourceMutationOptions<TData, TError, TVars, TCtx>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const apiErrorMessage = useApiErrorMessage()
  const notify = useNotify()

  return useMutation({
    ...mutation,
    onSuccess: async (data, variables) => {
      if (invalidate !== false) {
        const own = new Set(tagsOf(mutation.mutationKey))
        await Promise.all([
          queryClient.invalidateQueries({
            predicate: (query) => tagsOf(query.queryKey).some((tag) => own.has(tag)),
          }),
          ...(invalidate ?? []).map(async (queryKey) =>
            queryClient.invalidateQueries({ queryKey }),
          ),
        ])
      }
      if (successMessage) {
        notify({
          color: 'teal',
          title: t(($) => $.notifications.successTitle),
          message: successMessage,
        })
      }
      await onDone?.(data, variables)
    },
    onError: async (error) => {
      if (await handleError?.(error)) {
        return
      }
      notify({
        color: 'red',
        title: errorTitle,
        message:
          forbiddenMessage && isForbiddenError(error)
            ? forbiddenMessage
            : apiErrorMessage(
                error,
                t(($) => $.notifications.errorMessage),
              ),
      })
    },
  })
}

// tagsOf reads the operation tags a generated query or mutation key carries in its first element.
function tagsOf(key: QueryKey | undefined): readonly string[] {
  const head: unknown = key?.[0]
  if (typeof head !== 'object' || head === null || !('tags' in head) || !Array.isArray(head.tags)) {
    return []
  }
  return head.tags.filter((tag): tag is string => typeof tag === 'string')
}
