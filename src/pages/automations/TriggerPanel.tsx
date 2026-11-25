import { Stack } from '@mantine/core'

import type { TriggerDraft } from './definition.ts'
import { ExitEventsControl, GoalEventControl } from './EndEventControls.tsx'
import { CooldownControl, EnrollmentKeyControl, EntryPolicyControl } from './EntryControls.tsx'
import { TriggerControl } from './TriggerControl.tsx'
import { TriggerFilterControl } from './TriggerFilterControl.tsx'

// The Trigger node's panel: the event that enrolls a contact and its filter (ADR 0133), the Entry
// policy and its cooldown (ADR 0127), the Enrollment key (ADR 0165), and the exit and goal events
// (#751). Each control writes its part of the Trigger into the flow as it changes.
export function TriggerPanel({
  trigger,
  onEdit,
}: {
  trigger: TriggerDraft
  onEdit: (patch: Partial<TriggerDraft>) => void
}) {
  return (
    <Stack>
      <TriggerControl value={trigger.action} onChange={(action) => onEdit({ action })} />
      <TriggerFilterControl
        action={trigger.action}
        text={trigger.filterText}
        onChange={(filterText) => onEdit({ filterText })}
      />
      <EntryPolicyControl
        value={trigger.entryPolicy}
        onChange={(entryPolicy) => onEdit({ entryPolicy })}
      />
      {/* The cooldown applies under `again` alone (ADR 0127). */}
      {trigger.entryPolicy === 'again' && (
        <CooldownControl value={trigger.cooldown} onChange={(cooldown) => onEdit({ cooldown })} />
      )}
      <EnrollmentKeyControl
        action={trigger.action}
        value={trigger.enrollmentKey}
        onChange={(enrollmentKey) => onEdit({ enrollmentKey })}
      />
      <ExitEventsControl value={trigger.exits} onChange={(exits) => onEdit({ exits })} />
      <GoalEventControl value={trigger.goal} onChange={(goal) => onEdit({ goal })} />
    </Stack>
  )
}
