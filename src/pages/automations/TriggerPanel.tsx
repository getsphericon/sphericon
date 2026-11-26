import { Input, SegmentedControl, Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { AutomationTriggerInput } from '../../generated/site/types.gen.ts'
import { zDateTriggerInput, zEventTriggerInput } from '../../generated/site/zod.gen.ts'
import type { TriggerDraft } from './definition.ts'
import { ExitEventsControl, GoalEventControl } from './EndEventControls.tsx'
import { CooldownControl, EnrollmentKeyControl, EntryPolicyControl } from './EntryControls.tsx'
import { DateOccurrenceForm } from './MomentForms.tsx'
import type { StepValidator } from './stepValidator.ts'
import { TriggerControl } from './TriggerControl.tsx'
import { TriggerFilterControl } from './TriggerFilterControl.tsx'

// The kinds of Trigger, in the contract's order.
const TRIGGER_KINDS = [
  ...zEventTriggerInput.shape.kind.options,
  ...zDateTriggerInput.shape.kind.options,
]

// The Trigger node's panel: what enrolls a contact, an Event or a date. An Event Trigger has the
// event and its filter (ADR 0133), the Entry policy and its cooldown (ADR 0127) and the Enrollment
// key (ADR 0165); a date Trigger (ADR 0126) has the date form, the Wait until's, and none of those.
// Either has the exit and goal events (#751). Each control writes its part of the Trigger into the
// flow as it changes.
export function TriggerPanel({
  trigger,
  onEdit,
  validatorRef,
  checked,
  onChecked,
}: {
  trigger: TriggerDraft
  onEdit: (patch: Partial<TriggerDraft>) => void
  validatorRef: StepValidator
  // The Trigger was checked (on blur or on Save): its date's errors show as the panel opens.
  checked: boolean
  onChecked: () => void
}) {
  const { t } = useTranslation()
  const kinds: Record<AutomationTriggerInput['kind'], string> = {
    event: t(($) => $.automations.triggerKind.event),
    date: t(($) => $.automations.triggerKind.date),
  }

  return (
    <Stack>
      <Input.Wrapper label={t(($) => $.automations.triggerKindLabel)}>
        <SegmentedControl
          display="flex"
          data={TRIGGER_KINDS.map((kind) => ({ value: kind, label: kinds[kind] }))}
          value={trigger.kind}
          onChange={(picked) => {
            const kind = TRIGGER_KINDS.find((candidate) => candidate === picked)
            if (kind !== undefined) {
              onEdit({ kind })
            }
          }}
        />
      </Input.Wrapper>
      {trigger.kind === 'date' ? (
        <DateOccurrenceForm
          value={trigger.date}
          onChange={(date) => onEdit({ date })}
          validatorRef={validatorRef}
          checked={checked}
          onChecked={onChecked}
        />
      ) : (
        <>
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
            <CooldownControl
              value={trigger.cooldown}
              onChange={(cooldown) => onEdit({ cooldown })}
            />
          )}
          <EnrollmentKeyControl
            action={trigger.action}
            value={trigger.enrollmentKey}
            onChange={(enrollmentKey) => onEdit({ enrollmentKey })}
          />
        </>
      )}
      <ExitEventsControl value={trigger.exits} onChange={(exits) => onEdit({ exits })} />
      <GoalEventControl value={trigger.goal} onChange={(goal) => onEdit({ goal })} />
    </Stack>
  )
}
