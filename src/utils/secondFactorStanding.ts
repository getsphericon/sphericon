import type { SiteWorkspaceResource } from '../generated/site/types.gen.ts'

// Where the signed-in User stands under a Workspace's Two-factor requirement (ADR
// 0020): no deadline applies (no requirement, or they have a Second factor), they
// are in grace until a deadline, or their grace is over and the Workspace is
// withheld until they enroll.
export type SecondFactorStanding =
  | { kind: 'clear' }
  | { kind: 'grace'; endsAt: string }
  | { kind: 'blocked' }

export function secondFactorStanding(
  workspace: Pick<SiteWorkspaceResource, 'secondFactorGraceEndsAt'> | undefined,
  now: number,
): SecondFactorStanding {
  const endsAt = workspace?.secondFactorGraceEndsAt
  if (!endsAt) {
    return { kind: 'clear' }
  }
  return Date.parse(endsAt) <= now ? { kind: 'blocked' } : { kind: 'grace', endsAt }
}
