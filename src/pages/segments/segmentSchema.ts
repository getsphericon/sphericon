import { emptyGroup, placeInputSchemas } from '../../components/conditions/condition.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { SegmentValues } from './SegmentForm.tsx'

// A Segment is always a Condition: a new one starts as the empty Condition (matches all
// contacts); the contract has no null condition.

const blankSegment: SegmentValues = {
  name: '',
  condition: emptyGroup,
}

export const segmentSchema = resourceFormSchema(placeInputSchemas.segment, blankSegment)
