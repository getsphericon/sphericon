import type { UseMutationOptions } from '@tanstack/react-query'

import { useConfirmation } from './useConfirmation.tsx'
import { useResourceMutation } from './useResourceMutation.ts'

interface DeleteResourceOptions<TData, TError, TCtx> {
  // The generated delete mutation, called with { path: { workspaceId, id } }.
  mutation: UseMutationOptions<TData, TError, { path: { workspaceId: string; id: string } }, TCtx>
  workspaceId: string
  successMessage: string
  errorTitle: string
}

// Confirm, delete by id, refresh the list and toast: the delete action every resource list
// repeats. Returns the handler a row's delete button calls.
export function useDeleteResource<TData, TError, TCtx>({
  mutation,
  workspaceId,
  successMessage,
  errorTitle,
}: DeleteResourceOptions<TData, TError, TCtx>) {
  const confirmDelete = useConfirmation()
  const deletion = useResourceMutation({
    mutation,
    successMessage,
    errorTitle,
  })

  return (id: string) => {
    deletion.reset()
    confirmDelete({ onConfirm: () => deletion.mutate({ path: { workspaceId, id } }) })
  }
}
