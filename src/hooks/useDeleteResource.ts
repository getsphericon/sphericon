import type { QueryKey, UseMutationOptions } from '@tanstack/react-query'

import { useDeleteConfirmation } from './useDeleteConfirmation.tsx'
import { useResourceMutation } from './useResourceMutation.ts'

interface DeleteResourceOptions<TData, TError, TCtx> {
  // The generated delete mutation, called with { path: { workspaceId, id } }.
  mutation: UseMutationOptions<TData, TError, { path: { workspaceId: string; id: string } }, TCtx>
  workspaceId: string
  listQueryKey: QueryKey
  successMessage: string
  errorTitle: string
}

// Confirm, delete by id, refresh the list and toast: the delete action every resource list
// repeats. Returns the handler a row's delete button calls.
export function useDeleteResource<TData, TError, TCtx>({
  mutation,
  workspaceId,
  listQueryKey,
  successMessage,
  errorTitle,
}: DeleteResourceOptions<TData, TError, TCtx>) {
  const confirmDelete = useDeleteConfirmation()
  const deletion = useResourceMutation({
    mutation,
    invalidate: [listQueryKey],
    successMessage,
    errorTitle,
  })

  return (id: string) => {
    deletion.reset()
    confirmDelete({ onConfirm: () => deletion.mutate({ path: { workspaceId, id } }) })
  }
}
