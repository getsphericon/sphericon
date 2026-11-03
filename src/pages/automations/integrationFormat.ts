import type { SiteAutomationStepType } from '../../generated/site/types.gen.ts'

// The schema `format` that marks a property as the Integration a send step of the kind sends
// through: one per Channel, so the picker offers that Channel's Integrations only (ADR 0081).
export function integrationFormat(stepType: SiteAutomationStepType): string {
  return `integration-${stepType}`
}
