// displayValue is a JSON value as a person reads it: a string as it is, anything richer (a number,
// a flag, a list, an object) as compact JSON, and undefined for no value (absent, null or empty),
// which the caller shows its own way.
export function displayValue(value: unknown): string | undefined {
  if (value === undefined || value === null || value === '') {
    return undefined
  }
  return typeof value === 'string' ? value : JSON.stringify(value)
}
