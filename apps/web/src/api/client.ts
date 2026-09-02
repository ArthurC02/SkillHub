// Base URL of apps/platform (contracts/openapi/public.yaml `servers[0]`).
//
// Empty means same-origin, and that is the default ON PURPOSE: this value is
// baked in at build time, so the default has to be the shape that is correct
// when nobody sets it. Every way this app is served from a machine other than a
// developer's own is same-origin — the clean test mode (cmd/api serves
// apps/web/dist from its own process), a production deployment (ADR-018 E1: the
// SPA and the API share an origin), and `vite preview` under Playwright.
//
// It used to default to `http://localhost:8080`, the dev-server shape, and
// nothing on the clean-mode path set the variable. The failure had no red light
// anywhere: the page rendered, and the launcher's capability table ticked every
// row because it reads environment variables, not the bundle. The first symptom
// was `Failed to fetch` on a search — every request leaving the deployment for
// a port with nothing on it.
//
// The dev-server exception lives in apps/web/.env.development, which Vite loads
// only for `npm run dev`; `vite build` runs in production mode and never reads
// it. With DEV_CORS_ORIGIN on cmd/api (see README) the two-origin development
// flow is unchanged.
//
// Exported because one route is not fetched at all: the download content route
// is reached with a plain <a href>, so the browser saves the attachment itself
// (see api/packaging.ts).
export const API_BASE_URL: string = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "";

export class ApiError extends Error {
  status: number;
  body?: unknown;

  constructor(status: number, message: string, body?: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
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
      if (typeof body === "object" && body !== null && "error" in body) {
        const error = (body as { error?: unknown }).error;
        if (typeof error === "string") message = error;
      }
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
