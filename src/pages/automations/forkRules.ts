import * as z from 'zod'

import { zBranchStepInput, zSplitPathInput, zSplitStepInput } from '../../generated/site/zod.gen.ts'
import { maxItemsOf, minItemsOf } from '../../utils/bounds.ts'
import { branchConditionOf } from './definition.ts'
import type { ListedFork, SplitPathDraft } from './definition.ts'

// What the server asks of a fork (ADR 0124), checked in the editor before a Save sends it: the
// contract's bounds on a fork's paths and a Split path's weight, a Condition that tests something
// on each Branch path, and a Split's weights summing to 100, which is the server's rule alone.

const SPLIT_WHOLE = 100

// How many paths a fork lists, as the contract bounds them: a Branch's `otherwise` and unknown path
// aside.
const listedPaths = {
  branch: {
    min: minItemsOf(zBranchStepInput.shape.paths),
    max: maxItemsOf(zBranchStepInput.shape.paths),
  },
  split: {
    min: minItemsOf(zSplitStepInput.shape.paths),
    max: maxItemsOf(zSplitStepInput.shape.paths),
  },
}

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
        weight: z.union([z.literal(''), z.number()]).pipe(zSplitPathInput.shape.weight),
      }),
    ),
  }),
}

// The sum of a Split's weights, a blank one counting nothing.
export function weightSum(paths: SplitPathDraft[]): number {
  return paths.reduce((sum, path) => sum + (path.weight === '' ? 0 : path.weight), 0)
}

// What is wrong with a fork, the first thing found: a path forkSchemas refuses (a Branch path
// without a complete Condition, a Split path without a whole weight of at least 1), by its place, 0
// first, or weights that do not sum to 100.
export type ForkError =
  | { kind: 'condition'; index: number }
  | { kind: 'weight'; index: number }
  | { kind: 'sum'; sum: number }

export function forkError(step: ListedFork): ForkError | null {
  const parsed =
    step.kind === 'branch' ? forkSchemas.branch.safeParse(step) : forkSchemas.split.safeParse(step)
  const [issue] = parsed.error?.issues ?? []
  if (issue !== undefined) {
    // Every rule of forkSchemas is on a path: the issue's path is `paths.<index>.<field>`.
    const index = Number(issue.path[1])
    return step.kind === 'branch' ? { kind: 'condition', index } : { kind: 'weight', index }
  }
  if (step.kind === 'branch') {
    return null
  }
  const sum = weightSum(step.paths)
  return sum === SPLIT_WHOLE ? null : { kind: 'sum', sum }
}

// Whether a path can be added to the fork: it lists fewer than the contract allows.
export function canAddPath(step: ListedFork): boolean {
  return step.paths.length < listedPaths[step.kind].max
}

// Whether a path can be removed from the fork: it lists more than the contract asks for.
export function canRemovePath(step: ListedFork): boolean {
  return step.paths.length > listedPaths[step.kind].min
}
