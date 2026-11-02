import { act } from "react";
import { afterAll, beforeAll, expect, test } from "vitest";
import { SKILL, VERSION, SKILL_VERSIONS } from "../../../testing/fixtures/platform";
import { renderedApp, type App, type Reply } from "../../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});

afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

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
const OUTCOMES: { name: string; reply: Reply }[] = [
  { name: "null receipt", reply: { status: 201, json: null } },
  { name: "incomplete receipt", reply: { status: 201, json: {} } },
  {
    name: "another skill receipt",
    reply: { status: 201, json: { ...RECEIPT, skill_id: CREATED } },
  },
  {
    name: "server error with validation-shaped details",
    reply: { status: 503, json: FINDINGS },
  },
  { name: "lost connection", reply: "network-error" },
];

const VERSIONS_PATH = `/skills/${SKILL}/versions`;
const SUBMIT = "上傳成新版本";

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

function holdSessionRead(page: App) {
  let checking = false;
  let started!: () => void;
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    started = resolve;
  });
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  page.route("**/me", async () => {
    if (checking) {
      started();
      await gate;
    }
    return undefined;
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

function chosenName(page: App) {
  return (page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement).files?.[0]
    ?.name;
}

async function submitUploadForm(page: App) {
  const form = page.locator("form.version-upload").element() as HTMLFormElement;
  await act(async () => form.requestSubmit());
}

async function visibilityChange() {
  await act(async () => {
    document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
  });
}

for (const outcome of OUTCOMES) {
  test(`version upload retains an unknown outcome after ${outcome.name} and requires an explicit resend`, async () => {
    const page = app();
    const posts: string[] = [];
    let recovered = false;
    const session = holdSessionRead(page);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(
      (url) => url.pathname === VERSIONS_PATH,
      async ({ method, body }) => {
        if (method === "POST") {
          posts.push(await (body as File).text());
          if (posts.length > 1)
            return {
              status: 201,
              json: {
                ...RECEIPT,
                version_id: SECOND,
                version_number: 4,
                content_hash: "sha256:four",
              },
            };
          return outcome.reply;
        }
        if (!posts.length) return undefined;
        await gate;
        return recovered
          ? { json: currentVersions(posts.length > 1 ? 4 : 3) }
          : { status: 503, json: { error: "service unavailable" } };
      },
    );
    try {
      await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await chooseZip(page, "first.zip", "first bytes");
      await page.getByRole("button", { name: SUBMIT, exact: true }).click();
      const failure = page.getByRole("alert", { name: "上傳新版本結果", exact: true });
      const resend = page.getByRole("button", { name: "重新送出上傳", exact: true });
      await page.eventually(() => {
        expect(failure.text()).toContain("無法確認這份檔案是否已存成新版本");
        expect(failure.isFocused()).toBe(true);
        expect(resend.isDisabled()).toBe(true);
      });
      await chooseZip(page, "next.zip", "next bytes");
      const input = page.getByLabel("小工具 zip", { exact: true });
      await input.focus();
      release();
      await page.eventually(() => {
        expect(page.getByRole("alert").filter({ hasText: "暫時無法讀取版本脈絡" }).count()).toBe(1);
        expect(input.isFocused()).toBe(true);
        expect(failure.text()).toContain("這次送出的檔案：first.zip");
        expect(failure.text()).not.toContain("上傳被擋下");
      });
      expect(posts).toEqual(["first bytes"]);
      expect(await page.axeViolations("body")).toEqual([]);
      recovered = true;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() => {
        expect(page.getByRole("heading", { name: "PDF Summariser v2", exact: true }).count()).toBe(
          1,
        );
        expect(resend.isDisabled()).toBe(false);
        expect(failure.text()).toContain("無法確認");
      });
      const history = page.getByRole("link", { name: "另開版本與發佈", exact: true });
      expect(history.attribute("target")).toBe("_blank");
      expect(history.attribute("href")).toBe(`/skills/${SKILL}/versions/${VERSION}`);
      session.start();
      await visibilityChange();
      await session.pending;
      await expect(page.getByText("正在確認登入狀態…", { exact: true })).toHaveCount(1);
      await submitUploadForm(page);
      await page.eventually(() => {
        expect(failure.text()).toContain("無法確認這份檔案是否已存成新版本");
        expect(failure.text()).toContain("這次送出的檔案：first.zip");
        expect(resend.isDisabled()).toBe(true);
        expect(page.locator(`#${resend.attribute("aria-describedby")}`).text()).toBe(
          "登入狀態尚未確認；請先在頁首確認，再上傳新版本。",
        );
      });
      expect(posts).toEqual(["first bytes"]);
      session.restore();
      await page.eventually(() => {
        expect(page.locator(".auth-name").text()).toBe("tester");
        expect(resend.isDisabled()).toBe(false);
      });
      expect(chosenName(page)).toBe("next.zip");
      expect(posts).toEqual(["first bytes"]);
      await resend.click();
      await page.eventually(() => {
        expect(page.getByRole("status").filter({ hasText: "已存成 v4" }).count()).toBe(1);
        expect(
          page.getByRole("link", { name: "開啟 v4 並繼續驗證", exact: true }).attribute("href"),
        ).toBe(`/skills/${SKILL}/versions/${SECOND}`);
        expect(failure.count()).toBe(0);
      });
      expect(posts).toEqual(["first bytes", "next bytes"]);
    } finally {
      release();
      session.restore();
    }
  });
}

test("an identity check starting in the submit turn preserves the preceding unknown upload", async () => {
  const page = app();
  let posts = 0;
  const session = holdSessionRead(page);
  page.route(
    (url) => url.pathname === VERSIONS_PATH,
    ({ method }) => {
      if (method !== "POST") return undefined;
      posts += 1;
      return { status: 201, json: posts === 1 ? null : RECEIPT };
    },
  );
  try {
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await chooseZip(page, "first.zip", "first");
    await page.getByRole("button", { name: SUBMIT, exact: true }).click();
    const failure = page.getByRole("alert", { name: "上傳新版本結果", exact: true });
    const resend = page.getByRole("button", { name: "重新送出上傳", exact: true });
    await page.eventually(() => {
      expect(failure.text()).toContain("無法確認這份檔案是否已存成新版本");
      expect(resend.isDisabled()).toBe(false);
    });
    await chooseZip(page, "next.zip", "next");
    session.start();
    const form = page.locator("form.version-upload").element() as HTMLFormElement;
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      form.requestSubmit();
    });
    await session.pending;
    await page.eventually(() => {
      expect(page.getByText("正在確認登入狀態…", { exact: true }).count()).toBe(1);
      expect(
        page
          .getByRole("alert")
          .filter({ hasText: "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。" })
          .isFocused(),
      ).toBe(true);
      expect(failure.text()).toContain("無法確認這份檔案是否已存成新版本");
      expect(failure.text()).toContain("這次送出的檔案：first.zip");
      expect(resend.isDisabled()).toBe(true);
    });
    expect(posts).toBe(1);
    expect(await page.axeViolations("main")).toEqual([]);
    session.restore();
    await expect(resend).toBeEnabled();
    expect(posts).toBe(1);
    await resend.click();
    await page.eventually(() => {
      expect(page.getByRole("status").filter({ hasText: "已存成 v3" }).count()).toBe(1);
      expect(page.getByRole("link", { name: "開啟 v3 並繼續驗證", exact: true }).isFocused()).toBe(
        true,
      );
      expect(failure.count()).toBe(0);
    });
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
  test(`a version upload with ${refusal.name} preserves the confirmed refusal`, async () => {
    const page = app();
    let posts = 0;
    page.route(
      (url) => url.pathname === VERSIONS_PATH,
      ({ method }) => {
        if (method !== "POST") return undefined;
        posts += 1;
        return { status: refusal.status, json: refusal.body };
      },
    );
    await page.goto(`/skills/${SKILL}/versions/${VERSION}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await chooseZip(page, "rejected.zip", "rejected bytes");
    await page.getByRole("button", { name: SUBMIT, exact: true }).click();
    const failure = page.getByRole("alert", { name: "上傳新版本結果", exact: true });
    await page.eventually(() => {
      expect(failure.text()).toContain(refusal.message);
      expect(failure.isFocused()).toBe(true);
      expect(failure.text()).not.toContain("無法確認");
      expect(page.getByRole("link", { name: "另開版本與發佈", exact: true }).count()).toBe(0);
      expect(page.getByRole("button", { name: SUBMIT, exact: true }).isDisabled()).toBe(false);
    });
    expect(posts).toBe(1);
  });
}
