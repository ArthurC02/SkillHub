export const API_BASE_URL: string = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "";

export function internalAPIPath(value: string | undefined): string | undefined {
  if (!value?.startsWith("/") || value.startsWith("//")) return undefined;
  try {
    return new URL(value, window.location.origin).origin === window.location.origin
      ? value
      : undefined;
  } catch {
    return undefined;
  }
}

export class ApiError extends Error {
  status: number;
  body?: unknown;

  constructor(status: number, message: string, body?: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

const lastingReadFailures = new Set([403, 404]);

export function isLastingReadFailure(error: unknown): boolean {
  return error instanceof ApiError && lastingReadFailures.has(error.status);
}

function errorMessageFromBody(body: unknown, fallback: string): string {
  if (typeof body !== "object" || body === null || !("error" in body)) return fallback;
  const error = (body as { error?: unknown }).error;
  return typeof error === "string" ? error : fallback;
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    credentials: "include",
    ...init,
    headers: {
      Accept: "application/json",
      ...init?.headers,
    },
  });

  if (!response.ok) {
    let message = response.statusText;
    let body: unknown;
    try {
      body = await response.json();
      message = errorMessageFromBody(body, message);
    } catch {
      // Non-JSON error body; fall back to statusText.
    }
    throw new ApiError(response.status, message, body);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}
