import type { Page } from "@playwright/test";
import { platformResponse } from "../src/fixtures/platform";

/**
 * The browser half of the shared fixtures. vitest stubs `fetch`; here the same
 * `platformResponse` answers through `page.route`, so both tiers see one set of
 * bodies and neither keeps a copy that can drift from the other.
 *
 * A single catch-all rather than a route per endpoint, and it cannot be scoped
 * by path prefix: the platform owns /me, /policy/*, /runs/* and /test-cases/*
 * as well as /api/*. Anything let through reaches the preview server, which
 * answers index.html to every unknown path — a 200 full of HTML, parsed as
 * JSON, failing a long way from its cause. Documents, styles, scripts and fonts
 * are the app itself loading and have to go through.
 */
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
