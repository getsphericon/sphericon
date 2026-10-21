import { useQuery } from '@tanstack/react-query'

import {
  siteMembershipsListOptions,
  siteUserGetMeOptions,
} from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteMembershipRole } from '../generated/site/types.gen.ts'

// The signed-in user's role in a Workspace: their own Membership, found in the
// membership list (readable by any member). Undefined while loading or on error,
// so role-gated UI stays hidden until the role is known. The backend enforces the
// same gate (403) — this only avoids offering an action that would be refused.
export function useCurrentRole(workspaceId: string): SiteMembershipRole | undefined {
  const me = useQuery(siteUserGetMeOptions())
  const members = useQuery(siteMembershipsListOptions({ path: { workspaceId } }))
  return members.data?.find((member) => member.userId === me.data?.id)?.role
}

// Owners and Admins manage a Workspace's settings, integrations and Contact erasure; the API
// refuses everyone else with 403.
export function canManage(role: SiteMembershipRole | undefined) {
  return role === 'owner' || role === 'admin'
}

// Only an Owner manages the Workspace's Join domains; the section is not shown to
// anyone else.
export function canManageJoinDomains(role: SiteMembershipRole | undefined) {
  return role === 'owner'
}

// Only an Owner changes the Workspace's Brand kit (ADR 0040); every member reads it.
export function canManageBrandKit(role: SiteMembershipRole | undefined) {
  return role === 'owner'
}
