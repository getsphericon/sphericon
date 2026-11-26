import type { UseMutationOptions } from '@tanstack/react-query'

import {
  siteWorkspacesGetOptions,
  siteWorkspacesListQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from './useResourceMutation.ts'

interface WorkspaceSettingMutationOptions<TData, TError, TVars, TCtx> {
  // The generated mutation of the setting, e.g. siteWorkspacesSetConfirmedOptInMutation().
  mutation: UseMutationOptions<TData, TError, TVars, TCtx>
  successMessage: string
  errorTitle: string
  // What a Member is told: the server refuses everyone but an Owner and an Admin.
  forbiddenMessage: string
}

// useWorkspaceSettingMutation changes one of a Workspace's settings, such as its confirmed opt-in
// (ADR 0013) or its Engagement tracking default (ADR 0120), and refreshes the Workspace it answers
// with. A refused change (the Workspace changed since it was read, ADR 0119) still says why, and
// reads the Workspace again.
export function useWorkspaceSettingMutation<TData, TError, TVars, TCtx>(
  workspaceId: string,
  options: WorkspaceSettingMutationOptions<TData, TError, TVars, TCtx>,
) {
  return useResourceMutation({
    ...options,
    invalidate: [
      siteWorkspacesListQueryKey(),
      siteWorkspacesGetOptions({ path: { workspaceId } }).queryKey,
    ],
    rereadOnError: true,
  })
}
