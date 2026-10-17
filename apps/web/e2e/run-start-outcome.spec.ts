import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { SKILL, TEST_CASE, VERSION, RUN, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

function runReceiver(transport: string, onBody: (body: string) => void) {
  return createServer(async (request, response) => {
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://127.0.0.1:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
      "Access-Control-Allow-Methods": "POST",
      "Content-Type": "application/json",
    };
    if (request.method === "OPTIONS") {
      response.writeHead(204, headers).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    onBody(Buffer.concat(chunks).toString("utf8"));
    if (transport === "network") {
      response.writeHead(201, { ...headers, "Content-Length": "100" });
      response.write("{", () => response.destroy());
      return;
    }
    const bodies: Record<string, string> = {
      server: JSON.stringify({ error: "service unavailable" }),
      malformed: "{",
      incomplete: "{}",
      null: "null",
      valid: JSON.stringify({ run_id: RUN, status: "queued", provider: "unassigned" }),
    };
    response.writeHead(transport === "server" ? 503 : 201, headers).end(bodies[transport]);
  });
}

async function openPreflight(page: Page) {
  await page.goto(`/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await expect(
    page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }),
  ).toBeEnabled();
}

for (const transport of ["network", "server", "malformed", "incomplete", "null", "valid"]) {
  test(`${transport} Run acknowledgement preserves an honest result without automatic replay`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let confirms = 0;
    const writes: string[] = [];
    const receiver = runReceiver(transport, (body) => writes.push(body));
    await new Promise<void>((resolve, reject) => {
      receiver.once("error", reject);
      receiver.listen(0, "127.0.0.1", resolve);
    });
    try {
      await page.route("**/runs/preflight/confirm", (route) => {
        confirms += 1;
        return route.fulfill({ json: { confirmed: true, summary_hash: "hash-one" } });
      });
      await page.route(`**/skills/${SKILL}/runs`, (route) =>
        route.continue({
          url: `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/runs`,
        }),
      );
      await openPreflight(page);
      await page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }).click();
      const main = page.getByRole("main");
      if (transport === "valid") {
        const result = main.getByRole("link", { name: "查看這次試跑的結果", exact: true });
        await expect(result).toBeFocused();
        await expect(result).toHaveAttribute("href", `/runs/${RUN}`);
        await expect(main.getByRole("status")).toHaveText("已開始試跑。");
        await expect(main.getByRole("alert")).toHaveCount(0);
      } else {
        const alert = main.getByRole("alert");
        await expect(alert).toHaveText(
          "無法確認試跑是否已開始。請先查看試跑紀錄，再決定是否重新送出；再次送出可能建立另一筆試跑並使用額度。",
        );
        await expect(alert).toBeFocused();
        await expect(
          main.getByRole("button", { name: "核對後仍要再次開始試跑", exact: true }),
        ).toBeEnabled();
        await expect(main.getByText("已開始試跑。", { exact: true })).toHaveCount(0);
        await expect(
          main.getByRole("link", { name: "查看這次試跑的結果", exact: true }),
        ).toHaveCount(0);
      }
      expect(confirms).toBe(1);
      expect(writes).toHaveLength(1);
      expect(JSON.parse(writes[0])).toEqual({
        version_id: VERSION,
        test_case_id: TEST_CASE,
        confirmed_summary_hash: "hash-one",
      });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      await page.screenshot({
        path: test.info().outputPath("run-start-outcome.png"),
        fullPage: false,
      });
      if (transport !== "valid") {
        await main.getByRole("link", { name: "查看試跑紀錄", exact: true }).click();
        await expect(page.getByRole("heading", { name: "試跑活動", exact: true })).toBeVisible();
        expect(writes).toHaveLength(1);
        expect(confirms).toBe(1);
      }
    } finally {
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}

for (const confirmation of ["server", "malformed"]) {
  test(`${confirmation} confirmation failure is known not to have sent Run creation`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let confirms = 0;
    let starts = 0;
    await page.route("**/runs/preflight/confirm", (route) => {
      confirms += 1;
      return route.fulfill({
        status: confirmation === "server" ? 503 : 201,
        contentType: "application/json",
        body: confirmation === "server" ? JSON.stringify({ error: "service unavailable" }) : "{",
      });
    });
    await page.route(`**/skills/${SKILL}/runs`, (route) => {
      starts += 1;
      return route.fulfill({ status: 500, json: { error: "unexpected start" } });
    });
    await openPreflight(page);
    await page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }).click();
    const main = page.getByRole("main");
    await expect(main.getByRole("alert")).toHaveText(
      "權限確認未完成，尚未送出開始試跑要求。請重試權限確認。",
    );
    await expect(
      main.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }),
    ).toBeEnabled();
    await expect(main.getByRole("link", { name: "查看試跑紀錄", exact: true })).toHaveCount(0);
    expect(confirms).toBe(1);
    expect(starts).toBe(0);
  });
}

for (const blockedSession of [false, true]) {
  test(`${blockedSession ? "session-blocked" : "refused"} Run start is not an uncertain creation`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let checking = false;
    let writes = 0;
    await page.route("**/me", (route) =>
      checking
        ? route.fulfill({ status: 503, json: { error: "service unavailable" } })
        : route.fulfill({ json: platformResponse("/me").body }),
    );
    await page.route("**/runs/preflight/confirm", (route) => {
      writes += 1;
      return route.fulfill({ status: 403, json: { error: "closed beta" } });
    });
    await openPreflight(page);
    if (blockedSession) {
      checking = true;
      await page.evaluate(() =>
        document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
      );
      await expect(
        page.getByRole("button", { name: "重新確認登入狀態", exact: true }),
      ).toBeVisible();
    }
    await page.getByRole("button", { name: "我確認以上權限,開始試跑", exact: true }).click();
    const main = page.getByRole("main");
    if (blockedSession) {
      await expect(main.getByRole("alert")).toHaveText(
        "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
      );
      await expect(main.getByRole("alert")).toBeFocused();
    } else await expect(main.getByRole("alert")).toContainText("這個帳號還沒有封測邀請");
    await expect(main.getByRole("link", { name: "查看試跑紀錄", exact: true })).toHaveCount(0);
    await expect(
      main.getByRole("button", { name: "核對後仍要再次開始試跑", exact: true }),
    ).toHaveCount(0);
    expect(writes).toBe(blockedSession ? 0 : 1);
  });
}
