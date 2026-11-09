import { apiClient } from '@/api/client';
import type { ClientApiError } from '@/types/contracts';

// Shared helpers for the client API modules (issue #649). Frozen.

/** Base path of the client REST API (relative to /api/v1). */
export const CLIENT = '/client';

export { apiClient };

/** Extracts the {error, code, field} body from an axios error, if any. */
export function clientApiError(err: unknown): ClientApiError | null {
  const data = (err as { response?: { data?: unknown } })?.response?.data;
  if (data && typeof data === 'object' && 'error' in data) {
    return data as ClientApiError;
  }
  return null;
}

/** Error code of a client API error ("not_implemented", "name_taken", ...). */
export function clientApiErrorCode(err: unknown): string | undefined {
  return clientApiError(err)?.code;
}

/** HTTP status of an axios error. */
export function httpStatus(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status;
}
