import type {
  AutomationPathFigures,
  AutomationReport,
  AutomationStepFigures,
} from '../../generated/site/types.gen.ts'

// What a send Step's node shows: the messages it sent and skipped, over every version that had it.
type StepCounts = Pick<AutomationStepFigures, 'sent' | 'skipped'>
// What a fork's path shows: the Enrollments that took it and those among them that reached the
// goal, over every version that had it.
type PathCounts = Pick<AutomationPathFigures, 'enrollments' | 'goalReached'>

// The report's figures as the canvas reads them: a send Step's by its id, a path's by its fork's id
// and its own. The report lists only what something went through, so what it leaves out is zero.
export interface CanvasFigures {
  step: (stepId: string) => StepCounts
  path: (stepId: string, pathId: string) => PathCounts
}

const NO_MESSAGES: StepCounts = { sent: 0, skipped: 0 }
const NOT_TAKEN: PathCounts = { enrollments: 0, goalReached: 0 }

// A path's key: the report orders its paths by version, never by the canvas, so a path is found
// by its fork and its own id, never by its place.
function pathKey(stepId: string, pathId: string): string {
  return `${stepId}/${pathId}`
}

// canvasFigures reads the Automation's report for the canvas.
export function canvasFigures(report: AutomationReport): CanvasFigures {
  const steps = new Map(report.steps.map((figures) => [figures.stepId, figures]))
  const paths = new Map(
    report.paths.map((figures) => [pathKey(figures.stepId, figures.pathId), figures]),
  )
  return {
    step: (stepId) => steps.get(stepId) ?? NO_MESSAGES,
    path: (stepId, pathId) => paths.get(pathKey(stepId, pathId)) ?? NOT_TAKEN,
  }
}
