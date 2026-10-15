import type { Page } from "@playwright/test";

export async function setOffline(page: Page, offline: boolean) {
  const readiness = await page.evaluateHandle(
    (offline) => ({
      ready:
        navigator.onLine === !offline
          ? Promise.resolve()
          : new Promise<void>((resolve) =>
              window.addEventListener(offline ? "offline" : "online", () => resolve(), {
                once: true,
              }),
            ),
    }),
    offline,
  );
  try {
    await page.context().setOffline(offline);
    await readiness.evaluate(({ ready }) => ready);
  } finally {
    await readiness.dispose();
  }
}
