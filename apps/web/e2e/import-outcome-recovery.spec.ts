import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Request } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import type { ImportResult } from "../src/core/api/types";
import { platformResponse, SKILL, VERSION } from "../src/testing/fixtures/platform";
import { STATIC_KINDS, stubPlatform } from "./stub";

const RECEIPT: ImportResult = {
  shape: "skill",
  skills: [
    {
      path: "",
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 2,
      content_hash: "a".repeat(64),
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};

function traceImportRequests(page: Page) {
  const timeline: string[] = [];
  const tracked = new WeakSet<Request>();
  const record = (event: string) => (request: Request) => {
    if (request.url().includes("/skills/import/")) tracked.add(request);
    if (tracked.has(request))
      timeline.push(
        `${new Date().toISOString()} page ${event} ${request.method()} ${request.failure()?.errorText ?? ""}`,
      );
  };
  page.on("request", record("request"));
  page.on("requestfinished", record("requestfinished"));
  page.on("requestfailed", record("requestfailed"));
  return timeline;
}

async function importReceiver(mode: string, requests: string[], timeline: string[]) {
  const receiver = createServer(async (request, response) => {
    timeline.push(`${new Date().toISOString()} receiver got ${request.method}`);
    response.on("finish", () =>
      timeline.push(`${new Date().toISOString()} receiver answered ${response.statusCode}`),
    );
    const headers = {
      "Access-Control-Allow-Origin": request.headers.origin ?? "http://127.0.0.1:4173",
      "Access-Control-Allow-Credentials": "true",
      "Access-Control-Allow-Headers": "accept, content-type",
      "Content-Type": "application/json",
    };
    if (request.method === "OPTIONS") {
      response.writeHead(204, headers).end();
      return;
    }
    const chunks: Buffer[] = [];
    for await (const chunk of request) chunks.push(Buffer.from(chunk));
    requests.push(Buffer.concat(chunks).toString("utf8"));
    timeline.push(`${new Date().toISOString()} receiver read ${requests.at(-1)!.length} bytes`);
    if (requests.length > 1) {
      response.writeHead(201, headers).end(JSON.stringify(RECEIPT));
      return;
    }
    if (mode === "network") {
      response.writeHead(201, { ...headers, "Content-Length": "100" });
      response.write("{", () => response.destroy());
      return;
    }
    const bodies: Record<string, unknown> = {
      server: { ...RECEIPT, skills: [] },
      null: null,
      incomplete: { ...RECEIPT, excluded_components: undefined },
      empty: { ...RECEIPT, skills: [] },
      "submit-race": null,
    };
    response
      .writeHead(mode === "server" ? 503 : 201, headers)
      .end(mode === "malformed" ? "{" : JSON.stringify(bodies[mode]));
  });
  await new Promise<void>((resolve, reject) => {
    receiver.once("error", reject);
    receiver.listen(0, "127.0.0.1", resolve);
  });
  return receiver;
}

async function holdIdentityRead(page: Page) {
  let checking = false;
  let release!: () => void;
  let started!: () => void;
  let gate = Promise.resolve();
  let pending = Promise.resolve();
  await page.route("**/me", async (route) => {
    if (checking) {
      started();
      await gate;
    }
    return route.fulfill({ json: platformResponse("/me").body });
  });
  return {
    get pending() {
      return pending;
    },
    start: () => {
      checking = true;
      gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      pending = new Promise<void>((resolve) => {
        started = resolve;
      });
    },
    restore: () => {
      checking = false;
      release?.();
    },
  };
}

async function chooseDraft(page: Page, source: string, name: string) {
  if (source === "url") {
    await page.getByLabel("URL", { exact: true }).fill(`https://github.com/example/${name}`);
  } else {
    await page.getByLabel("上傳 zip", { exact: true }).check();
    await page.getByLabel("小工具 zip", { exact: true }).setInputFiles({
      name: `${name}.zip`,
      mimeType: "application/zip",
      buffer: Buffer.from(`${name} bytes`),
    });
  }
}

async function startCheckDuringSubmit(page: Page) {
  await page.evaluate(async () => {
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    document.querySelector<HTMLFormElement>(".import-source form")!.requestSubmit();
  });
}

async function checkLibraryWithoutLeaving(page: Page) {
  await expect(page.getByRole("link", { name: "另開資產庫核對", exact: true })).toHaveAttribute(
    "target",
    "_blank",
  );
  await expect(page.getByRole("link", { name: "另開資產庫核對", exact: true })).toHaveAttribute(
    "rel",
    "noopener noreferrer",
  );
  const popupPromise = page.waitForEvent("popup");
  await page.getByRole("link", { name: "另開資產庫核對", exact: true }).click();
  const popup = await popupPromise;
  await expect(popup.getByRole("heading", { name: "資產庫", exact: true })).toBeVisible();
  return popup;
}

async function stubPopupPlatform(page: Page) {
  await page.context().route("**/*", async (route) => {
    const kind = route.request().resourceType();
    if (kind !== "document" && STATIC_KINDS.has(kind)) {
      const response = await route.fetch({ maxRetries: 0 });
      return route.fulfill({ response });
    }
    if (!["xhr", "fetch"].includes(route.request().resourceType())) return route.continue();
    const result = platformResponse(route.request().url());
    return route.fulfill({ status: result.status, json: result.body });
  });
}

for (const [mode, event] of [
  ["null", "requestfinished"],
  ["network", "requestfailed"],
] as const) {
  test(`import diagnostics retain ${event} after transfer without tracking unrelated requests`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.goto("/");
    const timeline = traceImportRequests(page);
    const requests: string[] = [];
    const receiver = await importReceiver(mode, requests, timeline);
    const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/import`;
    await page.route("**/skills/import/probe", (route) => route.continue({ url: endpoint }));
    await page.route("**/other/probe", (route) => route.continue({ url: endpoint }));
    try {
      const isProbe = (request: Request) =>
        request.method() === "POST" && request.url() === endpoint;
      const terminal =
        event === "requestfailed"
          ? page.waitForEvent("requestfailed", isProbe)
          : page.waitForEvent("requestfinished", isProbe);
      const result = await page.evaluate(() =>
        fetch("/skills/import/probe", { method: "POST", body: "diagnostic bytes" })
          .then((response) => response.text())
          .catch(() => "network error"),
      );
      expect((await terminal).url()).toBe(endpoint);
      expect(result).toBe(mode === "network" ? "network error" : "null");
      const unrelated = page.waitForEvent(
        "requestfinished",
        (request) => request.method() === "POST" && request.url() === endpoint,
      );
      await page.evaluate(() =>
        fetch("/other/probe", { method: "POST", body: "unrelated bytes" }).then((response) =>
          response.text(),
        ),
      );
      await unrelated;
      expect(requests).toEqual(["diagnostic bytes", "unrelated bytes"]);
      const recorded = timeline.filter((line) => line.includes(" page "));
      expect(recorded).toHaveLength(2);
      expect(recorded[0]).toContain("page request POST ");
      expect(recorded[1]).toContain(`page ${event} POST `);
      if (event === "requestfailed") expect(recorded[1]).toMatch(/POST .+/);
    } finally {
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}

for (const source of ["url", "zip"]) {
  test(`${source} long import rejection keeps the verdict visible before its detailed findings`, async ({
    page,
  }) => {
    await stubPlatform(page);
    await page.setViewportSize({ width: 375, height: 900 });
    let requests = 0;
    await page.route(`**/skills/import/${source === "url" ? "url" : "upload"}`, (route) => {
      requests += 1;
      return route.fulfill({
        status: 422,
        json: {
          shape: "tree",
          skills: [],
          refused: Array.from({ length: 30 }, (_, index) => ({
            path: `skills/broken-${index + 1}`,
            findings: {
              errors: [
                {
                  severity: "error",
                  code: "name-missing",
                  message: "frontmatter 欄位 name 為必填",
                },
              ],
              warnings: [],
              infos: [],
            },
          })),
          excluded_components: [],
        },
      });
    });
    await page.goto("/workspace/import");
    const submit = page.getByRole("button", { name: "開始匯入", exact: true });
    await expect(submit).toBeEnabled();
    await chooseDraft(page, source, "first");
    await submit.click();
    const failure = page.getByRole("alert", { name: "匯入結果", exact: true });
    await expect(failure).toBeFocused();
    await expect(
      page.getByRole("heading", { name: "匯入失敗：這個來源沒有一個小工具進得來", exact: true }),
    ).toBeInViewport({ ratio: 1 });
    await expect(failure).toContainText("這個工作區沒有新增任何小工具，也沒有建立新版本");
    await expect(page.locator("main .finding-list > li")).toHaveCount(30);
    await expect(page.getByRole("link", { name: "另開資產庫核對", exact: true })).toHaveCount(0);
    expect(requests).toBe(1);
    await page.screenshot({ path: test.info().outputPath("import-refusal-verdict-phone.png") });
    await page.evaluate(() => scrollTo(0, 0));
    const report = await new AxeBuilder({ page }).include("main").analyze();
    await writeFile(test.info().outputPath("import-refusal-a11y.json"), JSON.stringify(report));
    expect(report.violations).toEqual([]);
    expect(report.incomplete).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: test.info().outputPath("import-refusal-phone.png"),
      fullPage: true,
    });
  });

  for (const mode of [
    "network",
    "server",
    "malformed",
    "null",
    "incomplete",
    "empty",
    "submit-race",
  ]) {
    test(`${source} import with ${mode} outcome keeps its draft and requires a deliberate resend after checking identity`, async ({
      page,
    }) => {
      await stubPlatform(page);
      await stubPopupPlatform(page);
      await page.setViewportSize({ width: 375, height: 900 });
      const session = await holdIdentityRead(page);
      const requests: string[] = [];
      const timeline = traceImportRequests(page);
      const receiver = await importReceiver(mode, requests, timeline);
      try {
        const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/import`;
        await page.route(`**/skills/import/${source === "url" ? "url" : "upload"}`, (route) =>
          route.continue({ url: endpoint }),
        );
        await page.goto("/workspace/import");
        await expect(page.locator(".auth-name")).toHaveText("tester");
        const submit = page.getByRole("button", { name: "開始匯入", exact: true });
        await expect(submit).toBeEnabled();
        await chooseDraft(page, source, "first");
        await submit.click();
        const failure = page.getByRole("alert", { name: "匯入結果", exact: true });
        await expect(failure).toContainText("無法確認是否已建立小工具或新版本");
        await expect(failure).toBeFocused();
        await expect(failure).toBeInViewport({ ratio: 1 });
        await expect(failure).not.toContainText("沒有新增任何小工具");
        expect(requests).toEqual(
          source === "url" ? ['{"url":"https://github.com/example/first"}'] : ["first bytes"],
        );
        await expect(submit).toBeEnabled();
        await chooseDraft(page, source, "next");
        if (mode === "submit-race") {
          session.start();
          await startCheckDuringSubmit(page);
          await expect(
            page
              .getByRole("alert")
              .filter({ hasText: "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。" }),
          ).toBeFocused();
        } else {
          const popup = await checkLibraryWithoutLeaving(page);
          session.start();
          await popup.close();
          await page.evaluate(() =>
            document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
          );
        }
        await session.pending;
        await expect(submit).toBeDisabled();
        await expect(submit).toHaveAccessibleDescription(
          "登入狀態尚未確認；請先在頁首確認，再送出匯入。",
        );
        await page
          .locator(".import-source form")
          .evaluate((form: HTMLFormElement) => form.requestSubmit());
        await expect(failure).toContainText("匯入結果尚未確認");
        expect(requests).toHaveLength(1);
        await page.evaluate(() => scrollTo(0, 0));
        const report = await new AxeBuilder({ page }).include("main").analyze();
        await writeFile(test.info().outputPath("import-outcome-a11y.json"), JSON.stringify(report));
        expect(report.violations).toEqual([]);
        expect(report.incomplete).toEqual([]);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        await page.screenshot({
          path: test.info().outputPath("import-outcome-phone.png"),
          fullPage: true,
        });
        session.restore();
        await expect(page.locator(".auth-name")).toHaveText("tester");
        await expect(submit).toBeEnabled();
        if (source === "url")
          await expect(page.getByLabel("URL", { exact: true })).toHaveValue(
            "https://github.com/example/next",
          );
        else
          expect(
            await page
              .getByLabel("小工具 zip", { exact: true })
              .evaluate(async (input: HTMLInputElement) => ({
                name: input.files?.[0]?.name,
                content: await input.files?.[0]?.text(),
              })),
          ).toEqual({ name: "next.zip", content: "next bytes" });
        expect(requests).toHaveLength(1);
        await submit.click();
        await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
        await expect(
          page.getByRole("link", { name: "開啟 v2 並繼續驗證", exact: true }),
        ).toHaveAttribute("href", `/skills/${SKILL}/versions/${VERSION}`);
        await expect(failure).toHaveCount(0);
        expect(requests).toEqual(
          source === "url"
            ? [
                '{"url":"https://github.com/example/first"}',
                '{"url":"https://github.com/example/next"}',
              ]
            : ["first bytes", "next bytes"],
        );
        await chooseDraft(page, source, "third");
        session.start();
        await startCheckDuringSubmit(page);
        await session.pending;
        await expect(
          page
            .getByRole("alert")
            .filter({ hasText: "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。" }),
        ).toBeFocused();
        await expect(failure).toHaveCount(0);
        await expect(page.getByRole("status").filter({ hasText: "匯入完成" })).toBeVisible();
        expect(requests).toHaveLength(2);
      } finally {
        timeline.push(`${new Date().toISOString()} teardown`);
        await test.info().attach("import-timeline", { body: timeline.join("\n") });
        session.restore();
        receiver.closeAllConnections();
        await new Promise<void>((resolve) => receiver.close(() => resolve()));
      }
    });
  }
}
