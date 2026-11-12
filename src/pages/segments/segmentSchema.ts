import { emptyGroup } from '../../components/conditions/condition.ts'
import { zSiteCreateSegmentInput } from '../../generated/site/zod.gen.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { SegmentValues } from './SegmentForm.tsx'

// A Segment is always a Condition: a stored segment without a condition loads as the empty
// Condition (matches all contacts); the contract has no null condition.

const blankSegment: SegmentValues = {
  name: '',
  condition: emptyGroup,
}

export const segmentSchema = resourceFormSchema(zSiteCreateSegmentInput, blankSegment)
