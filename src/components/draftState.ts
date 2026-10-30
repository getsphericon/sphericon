// What an Automation or a Rule can do with its draft (ADR 0065). A draft above a live version is
// pending, so it can be reverted. Publish makes the draft live, or resumes an inactive live version
// when there is no draft; it waits while the editor holds unsaved changes.
export function draftState({
  active,
  hasLive,
  hasDraft,
  dirty,
}: {
  active: boolean
  hasLive: boolean
  hasDraft: boolean
  dirty: boolean
}): { pending: boolean; publishable: boolean } {
  return {
    pending: hasLive && hasDraft,
    publishable: !dirty && (hasDraft || (hasLive && !active)),
  }
}
