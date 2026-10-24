import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { ME, RUN, SUGGESTIONS } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

function receiveDecision(fault: string, uncertain: boolean) {
  const writes: { decision: "accepted" | "rejected" }[] = [];
  const initialDecision = ["lost reply", "rejected input"].includes(fault) ? "accepted" : "pending";
  let saved = {
    ...SUGGESTIONS.suggestions[0],
    decision: initialDecision,
  };
  let release!: () => void;
  const responseGate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const server = createServer(async (request, response) => {
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://127.0.0.1:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
      "Access-Control-Allow-Methods": "PUT, OPTIONS",
      "Content-Type": "application/json",
    };
    if (request.method === "OPTIONS") {
      response.writeHead(204, headers).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    const intent = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    writes.push(intent);
    if (writes.length === 1) await responseGate;
    if (uncertain || writes.length > 1) saved = { ...saved, ...intent };
    if (writes.length === 1) {
      const receipts: Record<string, unknown> = {
        "lost reply": saved,
        "null receipt": null,
        "incomplete receipt": {},
        "opposite choice": {
          ...saved,
          decision: intent.decision === "accepted" ? "rejected" : "accepted",
        },
        "another suggestion": { ...saved, suggestion_id: "s2" },
      };
      if (Object.hasOwn(receipts, fault)) {
        response.writeHead(200, headers).end(JSON.stringify(receipts[fault]));
        return;
      }
      if (fault === "truncated reply") {
        response.writeHead(200, headers).end("{");
        return;
      }
      const status =
        fault === "server error"
          ? 503
          : fault === "rejected input"
            ? 400
            : fault === "missing suggestion"
              ? 404
              : 409;
      response
        .writeHead(status, headers)
        .end(JSON.stringify({ error: "這次決定被拒絕，請重新核對建議。" }));
      return;
    }
    response.writeHead(200, headers).end(JSON.stringify(saved));
  });
  return { server, writes, release, initialDecision, current: () => saved };
}

const REFUSAL_SAID: Record<string, string> = {
  "rejected input": "這個決定被拒絕。請重新讀取目前判定後再試。",
  "missing suggestion": "這個決定被拒絕。這個建議已經不在了，請重新讀取這份評估。",
  "applied conflict":
    "這個決定被拒絕。這個建議的狀態已經變了，可能已在別處做了決定或建成新版本；請重新讀取目前判定後再試。",
};

async function inspectDecision(page: Page, alert: Locator) {
  const report = await new AxeBuilder({ page }).analyze();
  await writeFile(test.info().outputPath("suggestion-decision-a11y.json"), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  await alert.scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath("suggestion-decision-viewport.png") });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
}

for (const [fault, decision, uncertain] of [
  ["server error", "accepted", true],
  ["lost reply", "rejected", true],
  ["truncated reply", "accepted", true],
  ["null receipt", "rejected", true],
  ["incomplete receipt", "accepted", true],
  ["opposite choice", "rejected", true],
  ["another suggestion", "accepted", true],
  ["rejected input", "rejected", false],
  ["missing suggestion", "accepted", false],
  ["applied conflict", "rejected", false],
] as const) {
  test(`a suggestion decision with ${fault} reports its actual certainty before an explicit resend`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.emulateMedia({ colorScheme: decision === "accepted" ? "light" : "dark" });
    await stubPlatform(page);
    let holdSession = false;
    let sessionReads = 0;
    let release!: () => void;
    const sessionGate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route("**/me", async (route) => {
      sessionReads++;
      if (holdSession) await sessionGate;
      return route.fulfill({ json: ME });
    });
    const receiver = receiveDecision(fault, uncertain);
    await new Promise<void>((resolve, reject) => {
      receiver.server.once("error", reject);
      receiver.server.listen(0, "127.0.0.1", resolve);
    });
    let reads = 0;
    await page.route(`**/runs/${RUN}/suggestions`, (route) => {
      reads++;
      return route.fulfill({ json: { ...SUGGESTIONS, suggestions: [receiver.current()] } });
    });
    const endpoint = `http://127.0.0.1:${(receiver.server.address() as AddressInfo).port}/decision`;
    let submissions = 0;
    await page.route("**/suggestions/s1/decision", async (route) => {
      submissions++;
      if (fault === "lost reply" && submissions === 1) {
        await route.fetch({ url: endpoint, maxRetries: 0 });
        return route.abort("failed");
      }
      return route.continue({ url: endpoint });
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const item = page.locator("li.suggestion");
      const submit = item.getByRole("button", {
        name: decision === "accepted" ? "接受" : "拒絕",
        exact: true,
      });
      await submit.click();
      await expect.poll(() => receiver.writes.length).toBe(1);
      const apply = page.getByRole("button", { name: /以已接受的.*項建議建立新版本/ });
      await expect(apply).toBeDisabled();
      receiver.release();
      const alert = item.getByRole("alert");
      await expect(alert).toContainText(uncertain ? "尚未確認" : REFUSAL_SAID[fault]);
      await expect(alert).not.toContainText("這次決定被拒絕，請重新核對建議。");
      await expect(alert).toBeFocused();
      await expect(item).not.toContainText("這個決定沒有記錄");
      const prior = receiver.initialDecision === "accepted" ? "已接受" : "尚未決定";
      await expect(item).toContainText(`${uncertain ? "上次讀取" : "目前"}：${prior}`);
      expect(receiver.writes).toEqual([{ decision }]);
      expect(reads).toBe(1);
      await inspectDecision(page, alert);
      if (uncertain) {
        await expect(apply).toBeDisabled();
        await expect(
          page.getByText(
            "改善建議的決定尚未確認；請等待目前操作完成，或重新送出未確認的決定後，再建立版本。",
            { exact: true },
          ),
        ).toBeVisible();
        expect(receiver.current().decision).toBe(decision);
        holdSession = true;
        const previous = sessionReads;
        await page.evaluate(() =>
          document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
        );
        await expect.poll(() => sessionReads).toBe(previous + 1);
        await submit.click();
        await expect(item.getByRole("alert").filter({ hasText: "這次操作尚未送出" })).toBeFocused();
        await expect(item.getByRole("alert").filter({ hasText: "尚未確認" })).toBeVisible();
        await expect(item).toContainText(`上次讀取：${prior}`);
        expect(receiver.writes).toEqual([{ decision }]);
        holdSession = false;
        release();
        await expect(page.getByRole("banner").getByRole("status")).toHaveCount(0);
        await submit.click();
        await expect(submit).toHaveAttribute("aria-pressed", "true");
        await expect(item.getByRole("alert")).toHaveCount(0);
        await expect(item).toContainText(`目前：${decision === "accepted" ? "已接受" : "已拒絕"}`);
        expect(receiver.writes).toEqual([{ decision }, { decision }]);
        expect(reads).toBe(2);
        if (decision === "rejected") await expect(apply).toBeDisabled();
        else await expect(apply).toBeEnabled();
      } else {
        expect(receiver.current().decision).toBe(receiver.initialDecision);
        await expect(page.locator("#suggestion-decisions-note")).toHaveCount(0);
        if (receiver.initialDecision === "accepted") await expect(apply).toBeEnabled();
      }
    } finally {
      release();
      receiver.release();
      await writeFile(
        test.info().outputPath("decision-http-observation.json"),
        JSON.stringify({
          submissions,
          writes: receiver.writes,
          current: receiver.current(),
          reads,
        }),
      );
      receiver.server.closeAllConnections();
      await new Promise<void>((resolve, reject) =>
        receiver.server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });
}
