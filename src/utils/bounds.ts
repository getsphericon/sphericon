import * as z from 'zod'

// zod gives a number without a declared bound an infinite one (a null only in its types).
function finite(bound: number | null): bound is number {
  return Number.isFinite(bound)
}

// The range a number of the contract declares (TypeSpec -> zod), for an input's min and max. The
// contract is the one source of a range: a number without a declared bound is a contract the form
// cannot follow, so it fails at once rather than falling back to a guess.
export function boundsOf(schema: { minValue: number | null; maxValue: number | null }): {
  min: number
  max: number
} {
  const { minValue: min, maxValue: max } = schema
  if (!finite(min) || !finite(max)) {
    throw new Error('the contract declares no range for this number')
  }
  return { min, max }
}

// The most items a list of the contract holds (TypeSpec @maxItems -> zod), for a form that adds
// rows up to it. zod keeps the bound only inside the schema, so it is read from the schema's JSON
// Schema; a list without one fails at once, as a range does.
export function maxItemsOf(schema: z.ZodArray): number {
  const { maxItems } = z.toJSONSchema(schema, { unrepresentable: 'any' })
  if (maxItems === undefined) {
    throw new Error('the contract declares no most items for this list')
  }
  return maxItems
}
