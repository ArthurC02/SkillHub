import { act } from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { ImportResult } from "../../../core/api/types";
import { platformResponse, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

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

const SESSION_NOT_READY = "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。";

function jsonText(status: number, text: string) {
  return new Response(text, { status, headers: { "Content-Type": "application/json" } });
}

function firstAnswer(mode: string) {
  const bodies: Record<string, Response> = {
    server: jsonText(503, JSON.stringify({ ...RECEIPT, skills: [] })),
    malformed: jsonText(201, "{"),
    null: jsonText(201, "null"),
    incomplete: jsonText(201, JSON.stringify({ ...RECEIPT, excluded_components: undefined })),
    empty: jsonText(201, JSON.stringify({ ...RECEIPT, skills: [] })),
    "submit-race": jsonText(201, "null"),
  };
  if (mode !== "network") return bodies[mode];
  const truncated = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode("{"));
      controller.error(new TypeError("terminated"));
    },
  });
  return new Response(truncated, { status: 201, headers: { "Content-Type": "application/json" } });
}

function receiveImports(mode: string) {
  const bodies: string[] = [];
  const harnessFetch = globalThis.fetch;
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    const { pathname } = new URL(String(input), "http://127.0.0.1");
    const isImport =
      (init?.method ?? "GET").toUpperCase() === "POST" &&
      /\/skills\/import\/(url|upload)$/.test(pathname);
    if (!isImport) return harnessFetch(input, init);
    const sent = init?.body;
    bodies.push(typeof sent === "string" ? sent : await (sent as Blob).text());
    return bodies.length > 1 ? jsonText(201, JSON.stringify(RECEIPT)) : firstAnswer(mode);
  });
  return bodies;
}

function holdIdentityRead(page: App) {
  let checking = false;
  let release!: () => void;
  let started!: () => void;
  let gate = Promise.resolve();
  let pending = Promise.resolve();
  page.route("**/me", async () => {
    if (checking) {
      started();
      await gate;
    }
    return { json: platformResponse("/me").body };
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

async function chooseZipWithoutNativeValidation(page: App, name: string, content: string) {
  const input = page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement;
  const file = new File([content], name, { type: "application/zip" });
  Object.defineProperty(input, "files", { configurable: true, value: [file] });
  input.removeAttribute("required");
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

async function chooseDraft(page: App, source: string, name: string) {
  if (source === "url") {
    await page.getByLabel("URL", { exact: true }).fill(`https://github.com/example/${name}`);
  } else {
    await page.getByLabel("上傳 zip", { exact: true }).check();
    await chooseZipWithoutNativeValidation(page, `${name}.zip`, `${name} bytes`);
  }
}

async function startCheckDuringSubmit(page: App) {
  const form = page.locator(".import-source form").element() as HTMLFormElement;
  await act(async () => {
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    form.requestSubmit();
  });
}

function describedBy(page: App, selector: string) {
  const id = page.locator(selector).attribute("aria-describedby") ?? "";
  return document.getElementById(id)?.textContent?.replace(/\s+/g, " ").trim();
}

function importWrites(page: App) {
  return page.requests.filter(
    (request) => request.method === "POST" && /\/skills\/import\//.test(request.url.pathname),
  );
}

for (const source of ["url", "zip"]) {
  test(`${source} long import rejection keeps the verdict visible before its detailed findings`, async () => {
    const page = app();
    page.route(`**/skills/import/${source === "url" ? "url" : "upload"}`, () => ({
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
    }));
    await page.goto("/workspace/import");
    const submit = page.getByRole("button", { name: "開始匯入", exact: true });
    await page.eventually(() => expect(submit.isDisabled()).toBe(false));
    await chooseDraft(page, source, "first");
    await submit.click();
    const failure = page.getByRole("alert", { name: "匯入結果", exact: true });
    await page.eventually(() => {
      expect(failure.isFocused()).toBe(true);
      expect(
        page
          .getByRole("heading", { name: "匯入失敗：這個來源沒有一個小工具進得來", exact: true })
          .count(),
      ).toBe(1);
      expect(failure.text()).toContain("這個工作區沒有新增任何小工具，也沒有建立新版本");
      expect(page.locator("main .finding-list > li").count()).toBe(30);
    });
    expect(page.getByRole("link", { name: "另開資產庫核對", exact: true }).count()).toBe(0);
    expect(importWrites(page)).toHaveLength(1);
    expect(await page.axeViolations("main")).toEqual([]);
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
    test(`${source} import with ${mode} outcome keeps its draft and requires a deliberate resend after checking identity`, async () => {
      const page = app();
      const session = holdIdentityRead(page);
      const requests = receiveImports(mode);
      try {
        await page.goto("/workspace/import");
        await page.eventually(() => expect(page.locator(".auth-name").text()).toBe("tester"));
        const submit = page.getByRole("button", { name: "開始匯入", exact: true });
        await page.eventually(() => expect(submit.isDisabled()).toBe(false));
        await chooseDraft(page, source, "first");
        await submit.click();
        const failure = page.getByRole("alert", { name: "匯入結果", exact: true });
        await page.eventually(() => {
          expect(failure.text()).toContain("無法確認是否已建立小工具或新版本");
          expect(failure.isFocused()).toBe(true);
        });
        expect(failure.text()).not.toContain("沒有新增任何小工具");
        expect(requests).toEqual(
          source === "url" ? ['{"url":"https://github.com/example/first"}'] : ["first bytes"],
        );
        expect(submit.isDisabled()).toBe(false);
        await chooseDraft(page, source, "next");
        const sessionNotReady = page.getByRole("alert").filter({ hasText: SESSION_NOT_READY });
        if (mode === "submit-race") {
          session.start();
          await startCheckDuringSubmit(page);
          await page.eventually(() => expect(sessionNotReady.isFocused()).toBe(true));
        } else {
          const library = page.getByRole("link", { name: "另開資產庫核對", exact: true });
          expect(library.attribute("target")).toBe("_blank");
          expect(library.attribute("rel")).toBe("noopener noreferrer");
          session.start();
          await act(async () => {
            document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
          });
        }
        await session.pending;
        await page.eventually(() => {
          expect(submit.isDisabled()).toBe(true);
          expect(describedBy(page, ".import-source form button[type=submit]")).toBe(
            "登入狀態尚未確認；請先在頁首確認，再送出匯入。",
          );
        });
        await act(async () => {
          (page.locator(".import-source form").element() as HTMLFormElement).requestSubmit();
        });
        await page.eventually(() => expect(failure.text()).toContain("匯入結果尚未確認"));
        expect(requests).toHaveLength(1);
        expect(await page.axeViolations("main")).toEqual([]);
        session.restore();
        await page.eventually(() => {
          expect(page.locator(".auth-name").text()).toBe("tester");
          expect(submit.isDisabled()).toBe(false);
        });
        if (source === "url")
          expect(page.getByLabel("URL", { exact: true }).value()).toBe(
            "https://github.com/example/next",
          );
        else {
          const input = page.getByLabel("小工具 zip", { exact: true }).element();
          const chosen = (input as HTMLInputElement).files?.[0];
          expect({ name: chosen?.name, content: await chosen?.text() }).toEqual({
            name: "next.zip",
            content: "next bytes",
          });
        }
        expect(requests).toHaveLength(1);
        await submit.click();
        await page.eventually(() => {
          expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1);
          expect(
            page.getByRole("link", { name: "開啟 v2 並繼續驗證", exact: true }).attribute("href"),
          ).toBe(`/skills/${SKILL}/versions/${VERSION}`);
          expect(failure.count()).toBe(0);
        });
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
        await page.eventually(() => {
          expect(sessionNotReady.isFocused()).toBe(true);
          expect(failure.count()).toBe(0);
          expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1);
        });
        expect(requests).toHaveLength(2);
      } finally {
        session.restore();
      }
    });
  }
}
