import { expect, test } from 'vitest'

import { stepId } from '../../test/payloads.ts'
import type { BranchStep, SplitStep } from './definition.ts'
import { canAddPath, canRemovePath, forkError, weightSum } from './forkRules.ts'

const VIP = JSON.stringify({
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
})

// A Branch whose listed paths hold these Conditions as the builder leaves them, with no Steps.
function branch(...conditions: string[]): BranchStep {
  return {
    id: stepId(20),
    kind: 'branch',
    paths: conditions.map((conditionText, i) => ({ id: stepId(30 + i), conditionText, steps: [] })),
    otherwise: { id: stepId(21), steps: [] },
    unknown: null,
  }
}

// A Split whose paths have these weights as typed, with no Steps.
function split(...weights: (number | '')[]): SplitStep {
  return {
    id: stepId(20),
    kind: 'split',
    paths: weights.map((weight, i) => ({ id: stepId(30 + i), weight, steps: [] })),
  }
}

test("a Branch's error is its first path without a complete Condition, by its place", () => {
  expect(forkError(branch(VIP, VIP))).toBeNull()
  expect(forkError(branch(VIP, ''))).toEqual({ kind: 'condition', index: 1 })
  expect(forkError(branch('{"kind":"group"', VIP))).toEqual({ kind: 'condition', index: 0 })
  expect(forkError(branch(VIP, '{"kind":"group","combinator":"and","entries":[]}'))).toEqual({
    kind: 'condition',
    index: 1,
  })
})

test("a Split's error is a weight that is blank or below 1, before weights that miss 100", () => {
  expect(forkError(split(30, 70))).toBeNull()
  expect(forkError(split(100, 0))).toEqual({ kind: 'weight', index: 1 })
  expect(forkError(split('', 50))).toEqual({ kind: 'weight', index: 0 })
  expect(forkError(split(50.5, 49.5))).toEqual({ kind: 'weight', index: 0 })
  expect(forkError(split(50, 49))).toEqual({ kind: 'sum', sum: 99 })
  expect(forkError(split(60, 41))).toEqual({ kind: 'sum', sum: 101 })
})

test("a Split's weights sum with a blank one counting nothing", () => {
  expect(weightSum(split('', 30, 20).paths)).toBe(50)
})

test('a path is added to a Branch up to ten with otherwise, and to a Split up to ten', () => {
  expect(canAddPath(branch(...Array<string>(8).fill(VIP)))).toBe(true)
  expect(canAddPath(branch(...Array<string>(9).fill(VIP)))).toBe(false)
  expect(canAddPath(split(...Array<number>(9).fill(10)))).toBe(true)
  expect(canAddPath(split(...Array<number>(10).fill(10)))).toBe(false)
})

test('a Branch keeps one path besides otherwise, and a Split two', () => {
  expect(canRemovePath(branch(VIP))).toBe(false)
  expect(canRemovePath(branch(VIP, VIP))).toBe(true)
  expect(canRemovePath(split(50, 50))).toBe(false)
  expect(canRemovePath(split(50, 25, 25))).toBe(true)
})
