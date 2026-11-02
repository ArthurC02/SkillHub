import { act } from "react";
import { afterAll, beforeAll, expect, test } from "vitest";
import { SKILL, VERSION, SKILL_VERSIONS } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});

afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const NEXT = "abcdef01-2345-4789-abcd-0123456789ab";
const FIRST_FILE = `${"package".repeat(35)}.zip`;
const SUBMIT = "上傳成新版本";
const SAVED = {
  skill_id: SKILL,
  version_id: NEXT,
  version_number: 3,
  content_hash: "sha256:next",
  duplicate: false,
  findings: { errors: [], warnings: [], infos: [] },
};
const CURRENT = {
  versions: [
    {
      version_id: NEXT,
      version_number: 3,
      content_hash: "sha256:next",
      created_at: "2026-10-15T00:00:00Z",
    },
    ...SKILL_VERSIONS.versions,
  ],
};

async function chooseZip(page: App, name: string, content: string) {
  const input = page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement;
  Object.defineProperty(input, "files", {
    configurable: true,
    value: [new File([content], name, { type: "application/zip" })],
  });
  input.removeAttribute("required");
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

async function openUpload(page: App) {
  await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
  await expect(page.locator(".auth-name")).toHaveText("tester");
  await chooseZip(page, FIRST_FILE, "first bytes");
  return page.getByLabel("小工具 zip", { exact: true });
}

async function chosenFile(input: ReturnType<App["getByLabel"]>) {
  const file = (input.element() as HTMLInputElement).files?.[0];
  return { name: file?.name, content: file ? await file.text() : undefined };
}

async function submitDirectly(page: App) {
  const form = page.locator(".version-upload").element();
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

const VERSIONS_PATH = `/skills/${SKILL}/versions`;

for (const source of ["skill", "versions"]) {
  test(`a confirmed version upload survives a failed ${source} reread and retains its next file through offline recovery`, async () => {
    const page = app();
    let uploaded = false;
    let recovered = false;
    let posts = 0;
    let rereads = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    async function rejectReread() {
      rereads += 1;
      await gate;
      return { status: 503, json: { error: "service unavailable" } };
    }
    page.route(`**/api/skills/${SKILL}`, () =>
      uploaded && !recovered && source === "skill" ? rejectReread() : undefined,
    );
    page.route(
      (url) => url.pathname === VERSIONS_PATH,
      ({ method }) => {
        if (method === "POST") {
          posts += 1;
          uploaded = true;
          return { status: 201, json: SAVED };
        }
        if (!uploaded) return undefined;
        return !recovered && source === "versions" ? rejectReread() : { json: CURRENT };
      },
    );
    try {
      const input = await openUpload(page);
      await page.getByRole("button", { name: SUBMIT, exact: true }).click();
      const status = page.getByRole("status").filter({ hasText: "已存成 v3" });
      const result = page.getByRole("link", { name: "開啟 v3 並繼續驗證", exact: true });
      await page.eventually(() => {
        expect(status.count()).toBe(1);
        expect(result.attribute("href")).toBe(`/skills/${SKILL}/versions/${NEXT}`);
        expect(result.isFocused()).toBe(true);
        expect(rereads).toBe(1);
        expect(page.locator("#version-facts-title").text()).toBe("v2，是否最新尚未確認");
      });
      await chooseZip(page, "next.zip", "next bytes");
      await input.focus();
      const submit = page.getByRole("button", { name: SUBMIT, exact: true });
      await expect(submit).toBeDisabled();
      await submitDirectly(page);
      release();
      await page.eventually(() => {
        expect(page.getByRole("alert").text()).toContain(
          source === "skill" ? "暫時無法讀取這個小工具" : "暫時無法讀取版本脈絡",
        );
        expect(input.isFocused()).toBe(true);
        expect(status.text()).toContain(`這次上傳的檔案：${FIRST_FILE}`);
        expect(submit.isDisabled()).toBe(true);
      });
      await submitDirectly(page);
      expect(await page.axeViolations("body")).toEqual([]);
      await result.click();
      await page.eventually(() => {
        expect(page.pathname()).toBe(`/skills/${SKILL}/versions/${NEXT}`);
        expect(input.count()).toBe(1);
      });
      expect(await chosenFile(input)).toEqual({ name: "next.zip", content: "next bytes" });
      recovered = true;
      await page.setOffline(true);
      await page.getByRole("button", { name: "再試一次", exact: true }).press("Enter");
      await page.eventually(() => {
        expect(page.getByRole("button", { name: /^(再試一次|重新讀取中…)$/ }).isDisabled()).toBe(
          true,
        );
        expect(
          page.getByRole("status").filter({ hasText: "等待連線以確認小工具與版本脈絡" }).text(),
        ).toBe("等待連線以確認小工具與版本脈絡；恢復連線後會繼續讀取。");
        expect(submit.isDisabled()).toBe(true);
      });
      const retained = (input.element() as HTMLInputElement).files?.[0];
      expect({ name: retained?.name, size: retained?.size }).toEqual({
        name: "next.zip",
        size: 10,
      });
      await page.setOffline(false);
      await page.eventually(() => {
        expect(page.getByRole("heading", { name: "PDF Summariser v3", exact: true }).count()).toBe(
          1,
        );
        expect(submit.isDisabled()).toBe(false);
        expect(status.text()).toContain(`這次上傳的檔案：${FIRST_FILE}`);
        expect(page.locator("main").isFocused()).toBe(true);
      });
      expect(await chosenFile(input)).toEqual({ name: "next.zip", content: "next bytes" });
      expect(posts).toBe(1);
    } finally {
      release();
      await page.setOffline(false);
    }
  });
}

for (const [source, status] of [
  ["skill", 401],
  ["skill", 410],
  ["versions", 401],
  ["versions", 403],
  ["versions", 404],
  ["versions", 200],
] as const) {
  test(`a ${source} reread returning ${status} removes protected cached upload controls`, async () => {
    const page = app();
    let uploaded = false;
    let posts = 0;
    const readsSkill = source === "skill";
    page.route(
      (url) =>
        url.pathname === (readsSkill ? `/api/skills/${SKILL}` : VERSIONS_PATH) && url.search === "",
      ({ method }) => {
        if (method !== "GET" || !uploaded) return undefined;
        return {
          status,
          json:
            status === 200
              ? { versions: [] }
              : { error: status === 401 ? "not authenticated" : "not found" },
        };
      },
    );
    page.route(
      (url) => url.pathname === VERSIONS_PATH,
      ({ method }) => {
        if (method !== "POST") return undefined;
        uploaded = true;
        posts += 1;
        return { status: 201, json: SAVED };
      },
    );
    const input = await openUpload(page);
    await page.getByRole("button", { name: SUBMIT, exact: true }).click();
    await page.eventually(() => {
      if (status === 401) expect(page.getByText("需要登入。").count()).toBeGreaterThan(0);
      else if (status === 200) expect(page.getByRole("alert").text()).toContain("無權檢視");
      else if (status === 410)
        expect(page.getByRole("alert").text()).toContain("這個小工具已從目錄下架");
      else expect(page.getByRole("alert").text()).toContain("暫時無法讀取版本脈絡");
      expect(input.count()).toBe(0);
      expect(page.getByRole("link", { name: "開啟 v3 並繼續驗證", exact: true }).count()).toBe(0);
    });
    expect(posts).toBe(1);
  });
}
