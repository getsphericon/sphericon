import { useQuery } from '@tanstack/react-query'

import { siteAuditListOptions } from '../generated/site/@tanstack/react-query.gen.ts'

// useAuditLogOffered tells whether the Audit log is offered to the User in the Workspace: a probe
// of one entry, which a plain member (403) and an Instance without an Enterprise license (402) are
// refused. What leads into the log (a row's history, the export) shows only where it holds; with
// `enabled` false (no row to offer it on) nothing is asked.
export function useAuditLogOffered(workspaceId: string, enabled = true): boolean {
  return useQuery({
    ...siteAuditListOptions({ path: { workspaceId }, query: { limit: 1 } }),
    enabled,
  }).isSuccess
}
