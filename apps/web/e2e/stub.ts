import type { Page } from "@playwright/test";
import { platformResponse } from "../src/fixtures/platform";

export async function stubPlatform(page: Page) {
  await page.route("**/*", (route, request) => {
    const kind = request.resourceType();
    if (kind === "document" || kind === "stylesheet" || kind === "script" || kind === "font") {
      return route.continue();
    }
    const { body, status } = platformResponse(request.url());
    const operatorPage = new URL(page.url()).pathname.startsWith("/admin");
    const me = new URL(request.url()).pathname === "/me";
    return route.fulfill({
      status,
      json: operatorPage && me ? { ...(body as object), operator: true } : (body as object),
    });
  });
}
