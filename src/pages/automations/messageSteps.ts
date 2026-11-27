import type { MessageStep } from '../../components/conditions/ConditionBuilder.tsx'
import { allSteps, sendsMessage, stepsBefore } from './definition.ts'
import type { FlowPath, FlowStep } from './definition.ts'

// messageStepsOf are the send Steps whose message the Step of the id may test (a Branch) or await
// (a Wait for event): those an Enrollment may have passed when it reaches it (ADR 0122), each named
// by its place on the canvas, its kind (infoOf) and an email's subject. An email's message is
// opened or clicked; another Channel's is only clicked.
export function messageStepsOf(
  infoOf: (step: FlowStep) => { label: string },
  root: FlowPath,
  id: string,
): MessageStep[] {
  const places = allSteps(root).map((step) => step.id)
  return stepsBefore(root, id)
    .filter((step) => sendsMessage(step))
    .map((step) => {
      const kind = infoOf(step).label
      const named =
        step.kind === 'email' && step.draft.subject !== '' ? `${kind}: ${step.draft.subject}` : kind
      return {
        id: step.id,
        label: `${places.indexOf(step.id) + 1}. ${named}`,
        opens: step.kind === 'email',
      }
    })
}
