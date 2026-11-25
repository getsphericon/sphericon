import * as z from 'zod'

import { allSteps, branchConditionOf, isFork } from './definition.ts'
import type { Flow, ForkStep, SplitPathDraft } from './definition.ts'

// What the server asks of a fork (ADR 0124), checked in the editor before a Save sends it: a
// Branch's every listed path has a Condition that tests something, a Split's weights are whole
// numbers of at least 1 summing to 100, and a fork has at most ten paths, `otherwise` counted, a
// Split at least two and a Branch one besides `otherwise`.

const PATHS_PER_FORK = 10
const SPLIT_WHOLE = 100

// The rules each fork's panel checks on its paths, on blur and on Save (docs/standards/ui.md).
// A Condition missing or still being typed reads as a missing value: the zod error map says so.
export const forkSchemas = {
  branch: z.object({
    paths: z.array(
      z.object({
        id: z.string(),
        conditionText: z.string().superRefine((text, context) => {
          if (branchConditionOf(text) === undefined) {
            // A Condition still being typed is as missing as none: the error map words it.
            context.addIssue({ code: 'custom', input: '' })
          }
        }),
      }),
    ),
  }),
  split: z.object({
    paths: z.array(
      z.object({
        id: z.string(),
        weight: z.union([z.literal(''), z.number()]).pipe(z.int().min(1)),
      }),
    ),
  }),
}

// The sum of a Split's weights, a blank one counting nothing.
export function weightSum(paths: SplitPathDraft[]): number {
  return paths.reduce((sum, path) => sum + (path.weight === '' ? 0 : path.weight), 0)
}

// What is wrong with a fork, the first thing found: a Branch path without a complete Condition (by
// its place, 0 first), a Split path without a whole weight of at least 1, or weights that do not
// sum to 100.
export type ForkError =
  | { kind: 'condition'; index: number }
  | { kind: 'weight'; index: number }
  | { kind: 'sum'; sum: number }

export function forkError(step: ForkStep): ForkError | null {
  if (step.kind === 'branch') {
    const index = step.paths.findIndex(
      (path) => branchConditionOf(path.conditionText) === undefined,
    )
    return index === -1 ? null : { kind: 'condition', index }
  }
  const index = step.paths.findIndex(
    (path) => path.weight === '' || !Number.isInteger(path.weight) || path.weight < 1,
  )
  if (index !== -1) {
    return { kind: 'weight', index }
  }
  const sum = weightSum(step.paths)
  return sum === SPLIT_WHOLE ? null : { kind: 'sum', sum }
}

// The forks of the flow, at any depth, that a Save would send with an error.
export function invalidForks(flow: Flow): ForkStep[] {
  return allSteps(flow.path).flatMap((step) =>
    isFork(step) && forkError(step) !== null ? [step] : [],
  )
}

// The paths a fork has, `otherwise` counted.
function pathCount(step: ForkStep): number {
  return step.kind === 'branch' ? step.paths.length + 1 : step.paths.length
}

// Whether a path can be added to the fork: it has fewer than ten.
export function canAddPath(step: ForkStep): boolean {
  return pathCount(step) < PATHS_PER_FORK
}

// Whether a path can be removed from the fork: a Branch keeps one besides `otherwise`, a Split two.
export function canRemovePath(step: ForkStep): boolean {
  return step.kind === 'branch' ? step.paths.length > 1 : step.paths.length > 2
}
