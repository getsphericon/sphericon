// The `demo` mise daemon (.mise.toml) exports everything the storefront reads; a missing
// variable means it was started some other way, so it fails at once instead of guessing.
export function requiredEnv(name: string): string {
  const value = process.env[name]
  if (!value) {
    throw new Error(`${name} is not set: the storefront runs under the mise demo daemon`)
  }

  return value
}
