import AxeBuilder from "@axe-core/playwright";
import { createServer } from "node:http";
import { writeFile } from "node:fs/promises";
import type { AddressInfo } from "node:net";
import { expect, test, type Page } from "@playwright/test";
import { SKILL, VERSION, SKILL_VERSIONS, platformResponse } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.use({ trace: "retain-on-failure" });

const CREATED = "abcdef01-2345-4789-abcd-0123456789ab";
const SECOND = "abcdef02-2345-4789-abcd-0123456789ab";
const RECEIPT = {
  skill_id: SKILL,
  version_id: CREATED,
  version_number: 3,
  content_hash: "sha256:three",
  duplicate: false,
  findings: { errors: [], warnings: [], infos: [] },
};
const FINDINGS = {
  errors: [{ severity: "error", code: "name-missing", message: "frontmatter 欄位 name 為必填" }],
  warnings: [],
  infos: [],
};
const OUTCOMES = [
  { name: "null receipt", status: 201, body: null },
  { name: "incomplete receipt", status: 201, body: {} },
  { name: "another skill receipt", status: 201, body: { ...RECEIPT, skill_id: CREATED } },
  { name: "server error with validation-shaped details", status: 503, body: FINDINGS },
  { name: "lost connection", status: 0, body: null },
];

function uploadReceiver(outcome: (typeof OUTCOMES)[number], posts: string[]) {
  return createServer(async (request, response) => {
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
    posts.push(Buffer.concat(chunks).toString("utf8"));
    if (posts.length > 1) {
      response.writeHead(201, headers).end(
        JSON.stringify({
          ...RECEIPT,
          version_id: SECOND,
          version_number: 4,
          content_hash: "sha256:four",
        }),
      );
    } else if (outcome.status) {
      response.writeHead(outcome.status, headers).end(JSON.stringify(outcome.body));
    } else {
      response.writeHead(201, { ...headers, "Content-Length": "100" });
      response.write("{", () => response.destroy());
    }
  });
}

function currentVersions(number: number) {
  return {
    versions: [
      ...(number === 4
        ? [
            {
              version_id: SECOND,
              version_number: 4,
              content_hash: "sha256:four",
              created_at: "2026-10-15T01:00:00Z",
            },
          ]
        : []),
      {
        version_id: CREATED,
        version_number: 3,
        content_hash: "sha256:three",
        created_at: "2026-10-15T00:00:00Z",
      },
      ...SKILL_VERSIONS.versions,
    ],
  };
}

async function holdSessionRead(page: Page) {
  let checking = false;
  let started!: () => void;
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    started = resolve;
  });
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/me", async (route) => {
    if (checking) {
      started();
      await gate;
    }
    return route.fulfill({ json: platformResponse("/me").body });
  });
  return {
    pending,
    start: () => {
      checking = true;
    },
    restore: () => {
      checking = false;
      release();
    },
  };
}

async function openUploadedVersionHistory(page: Page) {
  const history = page.getByRole("link", { name: "另開版本與發佈", exact: true });
  await expect(history).toHaveAttribute("target", "_blank");
  await expect(history).toHaveAttribute("href", `/skills/${SKILL}/versions/${VERSION}`);
  const [popup] = await Promise.all([page.context().waitForEvent("page"), history.click()]);
  await expect(
    popup.locator(".version-workspace-list").getByRole("link", { name: /^v3/ }),
  ).toBeVisible();
  return popup;
}

async function inspectUnknownUpload(page: Page) {
  const accessibility = await new AxeBuilder({ page }).analyze();
  await writeFile(
    test.info().outputPath("version-upload-unknown-a11y.json"),
    JSON.stringify(accessibility),
  );
  expect(accessibility.violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("version-upload-unknown.png") });
}

for (const outcome of OUTCOMES) {
  test(`version upload retains an unknown outcome after ${outcome.name} and requires an explicit resend`, async ({
    page,
    context,
  }) => {
    const posts: string[] = [];
    let recovered = false;
    const session = await holdSessionRead(page);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const receiver = uploadReceiver(outcome, posts);
    await new Promise<void>((resolve, reject) => {
      receiver.once("error", reject);
      receiver.listen(0, "127.0.0.1", resolve);
    });
    const endpoint = `http://127.0.0.1:${(receiver.address() as AddressInfo).port}/versions`;
    await context.route("**/*", async (route, request) => {
      if (request.url().startsWith(endpoint)) return route.continue();
      if (["document", "stylesheet", "script", "font"].includes(request.resourceType()))
        return route.continue();
      if (new URL(request.url()).pathname === `/skills/${SKILL}/versions`) {
        if (request.method() === "POST") {
          return route.continue({ url: endpoint });
        }
        if (posts.length) {
          await gate;
          return route.fulfill(
            recovered
              ? { json: currentVersions(posts.length > 1 ? 4 : 3) }
              : { status: 503, json: { error: "service unavailable" } },
          );
        }
      }
      const response = platformResponse(request.url());
      return route.fulfill({ status: response.status, json: response.body });
    });
    try {
      await page.setViewportSize({ width: 375, height: 900 });
      await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const input = page.getByLabel("小工具 zip", { exact: true });
      await input.setInputFiles({
        name: "first.zip",
        mimeType: "application/zip",
        buffer: Buffer.from("first bytes"),
      });
      await page.getByRole("button", { name: "上傳成新版本", exact: true }).click();
      const failure = page.getByRole("alert", { name: "上傳新版本結果", exact: true });
      await expect(failure).toContainText("無法確認這份檔案是否已存成新版本");
      await expect(failure).toBeFocused();
      const position = await failure.evaluate((element) => ({
        top: element.getBoundingClientRect().top,
        header: document.querySelector(".app-header")!.getBoundingClientRect().bottom,
      }));
      expect(position.top).toBeGreaterThanOrEqual(position.header);
      const resend = page.getByRole("button", { name: "重新送出上傳", exact: true });
      await expect(resend).toBeDisabled();
      await input.setInputFiles({
        name: "next.zip",
        mimeType: "application/zip",
        buffer: Buffer.from("next bytes"),
      });
      await input.focus();
      release();
      await expect(
        page.getByRole("alert").filter({ hasText: "暫時無法讀取版本脈絡" }),
      ).toBeVisible();
      await expect(input).toBeFocused();
      await expect(failure).toContainText("這次送出的檔案：first.zip");
      await expect(failure).not.toContainText("上傳被擋下");
      expect(posts).toEqual(["first bytes"]);
      await inspectUnknownUpload(page);
      recovered = true;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await expect(
        page.getByRole("heading", { name: "PDF Summariser v2", exact: true }),
      ).toBeVisible();
      await expect(resend).toBeEnabled();
      await expect(failure).toContainText("無法確認");
      const popup = await openUploadedVersionHistory(page);
      session.start();
      await popup.close();
      await page.evaluate(() =>
        document.dispatchEvent(new Event("visibilitychange", { bubbles: true })),
      );
      await session.pending;
      await expect(page.getByText("正在確認登入狀態…", { exact: true })).toBeVisible();
      await input.evaluate((element) => (element as HTMLInputElement).form!.requestSubmit());
      await expect(failure).toContainText("無法確認這份檔案是否已存成新版本");
      await expect(failure).toContainText("這次送出的檔案：first.zip");
      await expect(resend).toBeDisabled();
      await expect(resend).toHaveAccessibleDescription(
        "登入狀態尚未確認；請先在頁首確認，再上傳新版本。",
      );
      expect(posts).toEqual(["first bytes"]);
      session.restore();
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await expect(resend).toBeEnabled();
      await expect(input).toHaveValue(/(?:^|[\\/])next\.zip$/);
      expect(posts).toEqual(["first bytes"]);
      await resend.click();
      await expect(page.getByRole("status").filter({ hasText: "已存成 v4" })).toBeVisible();
      await expect(
        page.getByRole("link", { name: "開啟 v4 並繼續驗證", exact: true }),
      ).toHaveAttribute("href", `/skills/${SKILL}/versions/${SECOND}`);
      await expect(failure).toHaveCount(0);
      expect(posts).toEqual(["first bytes", "next bytes"]);
    } finally {
      release();
      session.restore();
      receiver.closeAllConnections();
      await new Promise<void>((resolve) => receiver.close(() => resolve()));
    }
  });
}

test("an identity check starting in the submit turn preserves the preceding unknown upload", async ({
  page,
}) => {
  await stubPlatform(page);
  await page.setViewportSize({ width: 375, height: 900 });
  let posts = 0;
  const session = await holdSessionRead(page);
  await page.route(`**/skills/${SKILL}/versions`, (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    posts += 1;
    return route.fulfill({ status: 201, json: posts === 1 ? null : RECEIPT });
  });
  try {
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    const input = page.getByLabel("小工具 zip", { exact: true });
    await input.setInputFiles({
      name: "first.zip",
      mimeType: "application/zip",
      buffer: Buffer.from("first"),
    });
    await page.getByRole("button", { name: "上傳成新版本", exact: true }).click();
    const failure = page.getByRole("alert", { name: "上傳新版本結果", exact: true });
    await expect(failure).toContainText("無法確認這份檔案是否已存成新版本");
    const resend = page.getByRole("button", { name: "重新送出上傳", exact: true });
    await expect(resend).toBeEnabled();
    await input.setInputFiles({
      name: "next.zip",
      mimeType: "application/zip",
      buffer: Buffer.from("next"),
    });
    session.start();
    await page.evaluate(async () => {
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      document.querySelector<HTMLFormElement>(".version-upload")!.requestSubmit();
    });
    await session.pending;
    await expect(page.getByText("正在確認登入狀態…", { exact: true })).toBeVisible();
    await expect(
      page
        .getByRole("alert")
        .filter({ hasText: "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。" }),
    ).toBeFocused();
    await expect(failure).toContainText("無法確認這份檔案是否已存成新版本");
    await expect(failure).toContainText("這次送出的檔案：first.zip");
    await expect(resend).toBeDisabled();
    expect(posts).toBe(1);
    await page.evaluate(() => window.scrollTo(0, 0));
    const report = await new AxeBuilder({ page })
      .include(".version-upload")
      .include('[role="alert"]')
      .analyze();
    const path = test.info().outputPath("pending-accessibility.json");
    await writeFile(path, JSON.stringify(report));
    await test.info().attach("pending-accessibility", { path, contentType: "application/json" });
    expect(report.violations).toEqual([]);
    expect(report.incomplete).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: test.info().outputPath("upload-session-pending-phone.png"),
      fullPage: true,
    });
    await failure.scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath("upload-session-feedback-phone.png") });
    session.restore();
    await expect(resend).toBeEnabled();
    expect(posts).toBe(1);
    await resend.click();
    await expect(page.getByRole("status").filter({ hasText: "已存成 v3" })).toBeVisible();
    await expect(page.getByRole("link", { name: "開啟 v3 並繼續驗證", exact: true })).toBeFocused();
    await expect(failure).toHaveCount(0);
    expect(posts).toBe(2);
  } finally {
    session.restore();
  }
});

for (const refusal of [
  {
    name: "oversized file",
    status: 413,
    body: { error: "request body too large" },
    message: "檔案超過上限，請縮小套件再上傳。",
  },
  {
    name: "valid validation details",
    status: 422,
    body: FINDINGS,
    message: "frontmatter 欄位 name 為必填",
  },
  {
    name: "unreadable validation details",
    status: 422,
    body: { ...FINDINGS, errors: [null] },
    message: "套件未通過驗證，但無法讀取檢查細節",
  },
]) {
  test(`a version upload with ${refusal.name} preserves the confirmed refusal`, async ({
    page,
  }) => {
    await stubPlatform(page);
    let posts = 0;
    await page.route(`**/skills/${SKILL}/versions`, (route) => {
      if (route.request().method() !== "POST") return route.fallback();
      posts += 1;
      return route.fulfill({ status: refusal.status, json: refusal.body });
    });
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await page.getByLabel("小工具 zip", { exact: true }).setInputFiles({
      name: "rejected.zip",
      mimeType: "application/zip",
      buffer: Buffer.from("rejected bytes"),
    });
    await page.getByRole("button", { name: "上傳成新版本", exact: true }).click();
    const failure = page.getByRole("alert", { name: "上傳新版本結果", exact: true });
    await expect(failure).toContainText(refusal.message);
    await expect(failure).toBeFocused();
    await expect(failure).not.toContainText("無法確認");
    await expect(page.getByRole("link", { name: "另開版本與發佈", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "上傳成新版本", exact: true })).toBeEnabled();
    expect(posts).toBe(1);
  });
}
