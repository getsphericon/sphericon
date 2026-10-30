import type { Permission, SiteWorkspaceResource } from '../generated/site/types.gen.ts'

// can tells whether the signed-in User's Membership holds a Permission in a Workspace
// (ADR 0061). The server lists the Permissions on the Workspace resource; the client reads
// them and keeps no Role table of its own. A Workspace that is not loaded yet holds none, so
// a gated action stays hidden until it is known. The server refuses the action anyway (403).
export function can(
  workspace: Pick<SiteWorkspaceResource, 'permissions'> | undefined,
  permission: Permission,
): boolean {
  return workspace?.permissions.includes(permission) ?? false
}
