import { expect, test } from 'vitest'
import { renderHook } from 'vitest-browser-react'

import { insertableKinds, useStepKinds } from './stepKinds.ts'

test('an End is offered only at the end of a path', async () => {
  const { result } = await renderHook(() => useStepKinds())

  const midPath = insertableKinds(result.current, false).map(({ kind }) => kind)
  const pathEnd = insertableKinds(result.current, true).map(({ kind }) => kind)

  expect(midPath).not.toContain('end')
  expect(midPath).toContain('wait')
  expect(pathEnd).toContain('end')
})
