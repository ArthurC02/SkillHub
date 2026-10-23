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

export class SessionNotReadyError extends Error {
  constructor() {
    super("這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。");
  }
}

export function isWriteResultUnknown(error: unknown): boolean {
  if (error instanceof SessionNotReadyError) return false;
  return !(error instanceof ApiError) || error.status >= 500;
}

const sessionlessPosts = new Set(["/auth/dev/login", "/auth/logout", "/api/skills/search"]);
let sessionReadinessCheck: (() => void) | undefined;

export function setSessionReadinessCheck(check: () => void) {
  sessionReadinessCheck = check;
}

const lastingReadFailures = new Set([403, 404]);

export function isLastingReadFailure(error: unknown): boolean {
  return error instanceof ApiError && lastingReadFailures.has(error.status);
}

export function isTransientReadFailure(error: unknown): boolean {
  return Boolean(
    error &&
    (!(error instanceof ApiError) ||
      error.status >= 500 ||
      error.status === 408 ||
      error.status === 429),
  );
}

export function canRetainLoadedPage(error: unknown, nextPageFailure: boolean): boolean {
  return (
    nextPageFailure &&
    !(error instanceof ApiError && error.status === 401) &&
    !isLastingReadFailure(error)
  );
}

export function serverReason(error: unknown): string | undefined {
  if (!(error instanceof ApiError) || error.status < 400 || error.status >= 500) return undefined;
  if (error.status === 401) return undefined;
  const reason = errorMessageFromBody(error.body, "");
  return reason === "" ? undefined : reason;
}

export function userFacingReason(error: unknown): string | undefined {
  if (!(error instanceof ApiError) || error.status === 401) return undefined;
  return /[㐀-鿿]/u.test(error.message) ? error.message : undefined;
}

function errorMessageFromBody(body: unknown, fallback: string): string {
  if (typeof body !== "object" || body === null || !("error" in body)) return fallback;
  const error = (body as { error?: unknown }).error;
  return typeof error === "string" ? error : fallback;
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  if (
    !["GET", "HEAD", "OPTIONS"].includes(method) &&
    !(method === "POST" && sessionlessPosts.has(path))
  ) {
    sessionReadinessCheck?.();
  }
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
