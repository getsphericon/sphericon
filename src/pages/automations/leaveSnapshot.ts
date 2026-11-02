import { graphToSteps } from './definition.ts'
import type { AutomationGraph } from './definition.ts'

// What Save would send, as text: the editor is unsaved while this differs from the saved one.
export function automationSnapshot(name: string, graph: AutomationGraph): string {
  return JSON.stringify([name.trim(), graphToSteps(graph)])
}
