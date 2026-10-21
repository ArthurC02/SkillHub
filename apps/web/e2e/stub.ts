import type { Page, Route } from "@playwright/test";
import { platformResponse } from "../src/testing/fixtures/platform";

export const STATIC_KINDS = new Set(["document", "stylesheet", "script", "font"]);
const STATIC_ATTEMPTS = 3;
const STATIC_RETRY_DELAY_MS = 250;

export async function serveStatic(route: Route, url?: string): Promise<void> {
  if (route.request().resourceType() === "document") return route.continue();
  const failures: string[] = [];
  for (let attempt = 1; attempt <= STATIC_ATTEMPTS; attempt += 1) {
    try {
      const response = await route.fetch(url ? { url } : {});
      return await route.fulfill({ response });
    } catch (error) {
      failures.push(error instanceof Error ? error.message : String(error));
    }
    if (attempt < STATIC_ATTEMPTS)
      await new Promise((resolve) => setTimeout(resolve, STATIC_RETRY_DELAY_MS));
  }
  await route.abort("failed");
  throw new Error(
    `${route.request().url()} could not be served after ${STATIC_ATTEMPTS} attempts: ${failures.join(" | ")}`,
  );
}

const TRANSIENT_NAVIGATION_FAILURE = "net::ERR_NO_BUFFER_SPACE";

export function retryTransientNavigation(page: Pick<Page, "goto">): void {
  const goto = page.goto.bind(page);
  page.goto = async (url, options) => {
    for (let attempt = 1; ; attempt += 1) {
      try {
        return await goto(url, options);
      } catch (error) {
        const transient = String(error).includes(TRANSIENT_NAVIGATION_FAILURE);
        if (!transient || attempt >= STATIC_ATTEMPTS) throw error;
      }
      await new Promise((resolve) => setTimeout(resolve, STATIC_RETRY_DELAY_MS));
    }
  };
}

export async function stubPlatform(page: Page) {
  retryTransientNavigation(page);
  await page.route("**/*", (route, request) => {
    if (STATIC_KINDS.has(request.resourceType())) return serveStatic(route);
    const { body, status } = platformResponse(request.url());
    const operatorPage = new URL(page.url()).pathname.startsWith("/admin");
    const me = new URL(request.url()).pathname === "/me";
    return route.fulfill({
      status,
      json: operatorPage && me ? { ...(body as object), operator: true } : (body as object),
    });
  });
}
