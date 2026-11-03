import type { UseMutationOptions } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { useConfirmation } from './useConfirmation.tsx'
import { useResourceMutation } from './useResourceMutation.ts'

interface DeleteResourceOptions<TData, TError, TCtx> {
  // The generated delete mutation, called with { path: { workspaceId, id } }.
  mutation: UseMutationOptions<TData, TError, { path: { workspaceId: string; id: string } }, TCtx>
  workspaceId: string
  successMessage: string
  errorTitle: string
  // What the deletion does and what to do first, in the resource's own words: no default.
  consequence: ReactNode
}

// Confirm, delete by id, refresh the list and toast: the delete action every resource list
// repeats. Returns the handler a row's delete button calls; passing the resource's name as the
// second argument asks for the additional (typed) confirmation.
export function useDeleteResource<TData, TError, TCtx>({
  mutation,
  workspaceId,
  successMessage,
  errorTitle,
  consequence,
}: DeleteResourceOptions<TData, TError, TCtx>) {
  const confirmDelete = useConfirmation()
  const deletion = useResourceMutation({
    mutation,
    successMessage,
    errorTitle,
  })

  return (id: string, typedName?: string) => {
    deletion.reset()
    confirmDelete({
      description: consequence,
      typedName,
      onConfirm: () => deletion.mutate({ path: { workspaceId, id } }),
    })
  }
}
