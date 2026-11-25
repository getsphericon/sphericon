import { useQueries } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'

import { siteAutomationsUnknownPath } from '../../generated/site/sdk.gen.ts'
import type { BranchConditionInput } from '../../generated/site/types.gen.ts'
import { allSteps, branchConditionOf } from './definition.ts'
import type { BranchStep, Flow } from './definition.ts'

// The complete Conditions of a Branch's listed paths: one still being typed tests nothing yet.
function conditionsOf(branch: BranchStep): BranchConditionInput[] {
  return branch.paths.flatMap((path) => branchConditionOf(path.conditionText) ?? [])
}

// useUnknownPaths keeps each Branch's unknown path (ADR 0124) to what its Conditions need: the
// server answers whether they can be unknown for a Contact, as Publish decides it (the editor never
// computes it). A Branch that needs one gains it, empty; one that no longer does loses it, at once
// when it is empty, else when `confirmRemoval` is accepted, which is asked once until the need
// changes again. A version the editor cannot change is left as it is.
export function useUnknownPaths({
  workspaceId,
  flow,
  editable,
  setUnknown,
  confirmRemoval,
}: {
  workspaceId: string
  flow: Flow
  editable: boolean
  setUnknown: (branchId: string, present: boolean) => void
  confirmRemoval: (branch: BranchStep) => void
}) {
  const branches = editable
    ? allSteps(flow.path).filter((step): step is BranchStep => step.kind === 'branch')
    : []
  const asked = branches.map((branch) => conditionsOf(branch))
  const answers = useQueries({
    queries: asked.map((conditions) => ({
      queryKey: ['automationsUnknownPath', workspaceId, conditions],
      queryFn: async () => {
        const { data } = await siteAutomationsUnknownPath({
          path: { workspaceId },
          body: { conditions },
          throwOnError: true,
        })
        return data.needed
      },
      enabled: conditions.length > 0,
      staleTime: Number.POSITIVE_INFINITY,
    })),
  })
  // Whether each Branch needs an unknown path, undefined while the server has not said: a Branch
  // whose Conditions are all still being typed needs none.
  const needs = branches.map((branch, i) =>
    (asked[i]?.length ?? 0) === 0 ? false : answers[i]?.data,
  )
  // The Branches already asked about losing a path that holds Steps.
  const confirming = useRef(new Set<string>())

  const pending = branches.flatMap((branch, i) => {
    const needed = needs[i]
    return needed === undefined || needed === (branch.unknown !== null) ? [] : [{ branch, needed }]
  })
  useEffect(() => {
    for (const { branch, needed } of pending) {
      if (needed) {
        confirming.current.delete(branch.id)
        setUnknown(branch.id, true)
      } else if (branch.unknown?.steps.length === 0) {
        setUnknown(branch.id, false)
      } else if (!confirming.current.has(branch.id)) {
        confirming.current.add(branch.id)
        confirmRemoval(branch)
      }
    }
  })
}
