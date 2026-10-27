/**
 * Legacy static search form stores `method` as a string or a string[].
 * The default is `[]` (see DEFAULT_FORM). An empty array is truthy, but the
 * transactions API binds `method` as a string — `"method": []` is a 400.
 * One value uses `method`; several use `methods` (SQL IN), matching the
 * dynamic mapping branch.
 */
export function assignLegacyMethodFilter(
  filter: Record<string, unknown>,
  method: unknown,
): void {
  if (Array.isArray(method)) {
    if (method.length === 1) filter.method = method[0]
    else if (method.length > 1) filter.methods = method
    return
  }
  if (method) filter.method = method
}
