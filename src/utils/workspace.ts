import { queryOptions } from '@tanstack/react-query'

import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../generated/site/types.gen.ts'

// workspaceOptions reads one Workspace of the signed-in User from the Workspace list the console
// already holds: the list's query, whose select picks the row, so a mutation that refreshes the
// list refreshes every reader of the Workspace. undefined when the list does not hold it.
export function workspaceOptions(workspaceId: string) {
  return queryOptions({
    ...siteWorkspacesListOptions(),
    select: (workspaces: SiteWorkspaceResource[]) => workspaces.find((w) => w.id === workspaceId),
  })
}
