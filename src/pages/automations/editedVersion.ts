import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'

// The key that remounts the editor when the version it edits (the draft, else the live one; ADR
// 0065) is replaced: the first save above the live version, a publish or a revert.
export function editedVersionKey(automation: SiteAutomationResource): string {
  return automation.draft === null ? 'live' : `draft-${automation.draft.number}`
}
