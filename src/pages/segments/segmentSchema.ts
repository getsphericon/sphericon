import { emptyGroup } from '../../components/conditions/condition.ts'
import { zCreateSegmentInput, zSegmentConditionInput } from '../../generated/site/zod.gen.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { SegmentValues } from './SegmentForm.tsx'

// A Segment is always a Condition: a stored segment without a condition loads as the empty
// Condition (matches all contacts); the contract has no null condition.

const blankSegment: SegmentValues = {
  name: '',
  condition: emptyGroup,
}

// The generated create input reaches the Condition through a lazy schema typed any, so it is given
// the Segment's Condition schema by name, which types it.
export const segmentSchema = resourceFormSchema(
  zCreateSegmentInput.extend({ condition: zSegmentConditionInput }),
  blankSegment,
)
