import type { Page } from "@playwright/test";
import { platformResponse } from "../src/fixtures/platform";

export async function stubPlatform(page: Page) {
  await page.route("**/*", (route, request) => {
    const kind = request.resourceType();
    if (kind === "document" || kind === "stylesheet" || kind === "script" || kind === "font") {
      return route.continue();
    }
    const { body, status } = platformResponse(request.url());
    return route.fulfill({ status, json: body as object });
  });
}
