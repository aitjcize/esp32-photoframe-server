// getApiError extracts a human-readable message from an Axios/API error,
// preferring the server's JSON {"error": ...} body and falling back to the
// generic error message. Centralizes the `e.response?.data?.error || e.message`
// pattern that was duplicated across stores and components.
export function getApiError(e: unknown, fallback = 'Request failed'): string {
  const err = e as
    | { response?: { data?: { error?: string } }; message?: string }
    | undefined;
  return err?.response?.data?.error || err?.message || fallback;
}
