/**
 * ApiError wraps a failed API response, carrying both the HTTP status code
 * and the parsed error body so callers can discriminate between e.g. 404 and
 * 500 without inspecting raw response objects.
 *
 * Usage in a queryFn:
 *   const { data, error, response } = await api.GET("/things/{id}", ...)
 *   if (error) throw new ApiError(response.status, error)
 *
 * Usage in a component:
 *   if (isError && isNotFound(error)) return <p>Not found.</p>
 */
export class ApiError extends Error {
  public readonly status: number;
  public readonly body: unknown;

  constructor(status: number, body: unknown) {
    const typed = body as { message?: string; error?: string } | null;
    const msg =
      typed?.message ??
      typed?.error ??
      `Request failed with status ${status}`;
    super(msg);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

/** Returns true when the error is a 404 Not Found response. */
export function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

/** Returns true when the error is a 409 Conflict response. */
export function isConflict(error: unknown): boolean {
  return error instanceof ApiError && error.status === 409;
}

/** Returns true when the error is a 400 Bad Request response. */
export function isBadRequest(error: unknown): boolean {
  return error instanceof ApiError && error.status === 400;
}

/** Returns true when the error is a 5xx server-side response. */
export function isServerError(error: unknown): boolean {
  return error instanceof ApiError && error.status >= 500;
}

/**
 * Extracts a human-readable message from any thrown value.
 * Falls back to a generic string so callers never need to null-check.
 */
export function getErrorMessage(error: unknown, fallback = "An unexpected error occurred."): string {
  if (error instanceof ApiError) return error.message;
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  return fallback;
}
