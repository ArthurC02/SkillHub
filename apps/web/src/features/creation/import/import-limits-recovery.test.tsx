import { act } from "react";
import { expect, test } from "vitest";
import { IMPORT_LIMITS, SKILL, VERSION } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

const draft = "https://github.com/example/private-draft";
const imported = {
  shape: "skill",
  skills: [
    {
      skill_id: SKILL,
      version_id: VERSION,
      version_number: 2,
      content_hash: "a".repeat(64),
      path: "skill",
      duplicate: false,
      findings: { errors: [], warnings: [], infos: [] },
    },
  ],
  refused: [],
  excluded_components: [],
};

async function chooseZipWithoutNativeValidation(page: App, name: string, content: string) {
  const input = page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement;
  const file = new File([content], name, { type: "application/zip" });
  Object.defineProperty(input, "files", { configurable: true, value: [file] });
  input.removeAttribute("required");
  await act(async () => {
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

async function submitForm(page: App) {
  const form = page.locator(".import-source form").element();
  await act(async () => {
    form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
}

function countWrites(page: App, source: string) {
  page.route(`**/skills/import/${source}`, () => ({ status: 201, json: imported }));
  return () =>
    page.requests.filter(
      (request) => request.method === "POST" && request.url.pathname.endsWith(`/import/${source}`),
    ).length;
}

for (const failure of ["server", "network", "malformed"]) {
  for (const source of ["url", "upload"]) {
    test(`${failure} import limits retain the ${source} draft and retry only the read on a phone`, async () => {
      const page = app();
      let failed = true;
      page.route("**/skills/import/limits", () => {
        if (!failed) return { json: IMPORT_LIMITS };
        if (failure === "network") return "network-error";
        if (failure === "malformed") return { json: { allowed_hosts: "github.com" } };
        return { status: 503, json: { error: "service unavailable" } };
      });
      const writes = countWrites(page, source);
      await page.goto("/workspace/import");
      const rules = page.getByRole("region", { name: "匯入前檢查", exact: true });
      await page.eventually(() => {
        expect(rules.locator("[role=alert]").text()).toBe("無法讀取匯入限制。請重新讀取後再送出。");
        expect(page.locator(".auth-name").text()).toBe("tester");
      });
      expect(rules.text()).not.toContain("來源限");
      expect(rules.locator("[role=status]").count()).toBe(0);
      if (source === "url")
        await page.getByRole("textbox", { name: "URL", exact: true }).fill(draft);
      else {
        await page.getByRole("radio", { name: "上傳 zip", exact: true }).check();
        await chooseZipWithoutNativeValidation(page, "private.zip", "zip draft");
      }
      const submit = page.getByRole("button", { name: "開始匯入", exact: true });
      await page.eventually(() => {
        expect(submit.isDisabled()).toBe(true);
        expect(submit.attribute("aria-describedby")).toBe("import-rules-unavailable");
        expect(page.locator("#import-rules-unavailable").text()).toBe(
          "匯入限制尚未確認，請先讀取成功後再送出。",
        );
      });
      await submitForm(page);
      expect(writes()).toBe(0);
      expect(await page.axeViolations("body")).toEqual([]);
      failed = false;
      await rules.locator("button").filter({ hasText: "再試一次" }).click();
      await page.eventually(() => {
        expect(page.getByRole("heading", { name: "匯入前檢查", exact: true }).isFocused()).toBe(
          true,
        );
        expect(rules.text()).toContain("zip 最大 10 MB");
        expect(rules.text()).toContain("來源限 github.com");
        expect(rules.locator("[role=alert]").count()).toBe(0);
        expect(submit.isDisabled()).toBe(false);
      });
      if (source === "url")
        expect(page.getByRole("textbox", { name: "URL", exact: true }).value()).toBe(draft);
      else
        expect(
          (page.getByLabel("小工具 zip", { exact: true }).element() as HTMLInputElement).files?.[0]
            ?.name,
        ).toBe("private.zip");
      expect(writes()).toBe(0);
      await submit.click();
      await page.eventually(() =>
        expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1),
      );
      expect(writes()).toBe(1);
    });
  }
}

test("pending import limits keep an editable draft without submitting it", async () => {
  const page = app();
  let release!: () => void;
  let started!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const readStarted = new Promise<void>((resolve) => {
    started = resolve;
  });
  page.route("**/skills/import/limits", async () => {
    started();
    await gate;
    return { json: IMPORT_LIMITS };
  });
  const writes = countWrites(page, "url");
  try {
    await page.goto("/workspace/import");
    await readStarted;
    await page.eventually(() => {
      expect(page.locator(".auth-name").text()).toBe("tester");
      expect(page.getByText("正在讀取匯入限制…", { exact: true }).count()).toBe(1);
    });
    const input = page.getByRole("textbox", { name: "URL", exact: true });
    await input.fill(draft);
    const submit = page.getByRole("button", { name: "開始匯入", exact: true });
    expect(submit.isDisabled()).toBe(true);
    await submitForm(page);
    expect(writes()).toBe(0);
    release();
    await page.eventually(() => expect(submit.isDisabled()).toBe(false));
    expect(input.value()).toBe(draft);
    expect(writes()).toBe(0);
    await submit.click();
    await page.eventually(() =>
      expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1),
    );
    expect(writes()).toBe(1);
  } finally {
    release();
  }
});

test("an offline return withholds cached import limits until the paused read resumes", async () => {
  const page = app();
  const writes = countWrites(page, "url");
  await page.goto("/workspace/import");
  const rules = page.getByRole("region", { name: "匯入前檢查", exact: true });
  await page.eventually(() => expect(rules.text()).toContain("zip 最大 10 MB"));
  await page.getByRole("link", { name: "Skill Hub", exact: true }).click();
  await page.eventually(() => {
    expect(page.getByRole("heading", { name: "匯入小工具", exact: true }).count()).toBe(0);
    expect(page.locator("main h1").count()).toBe(1);
  });
  try {
    await page.setOffline(true);
    await page.goBack();
    await page.eventually(() => {
      expect(rules.locator("[role=status]").text()).toBe("等待連線以讀取匯入限制…");
      expect(rules.text()).not.toContain("zip 最大 10 MB");
    });
    const input = page.getByRole("textbox", { name: "URL", exact: true });
    await input.fill(draft);
    const submit = page.getByRole("button", { name: "開始匯入", exact: true });
    expect(submit.isDisabled()).toBe(true);
    expect(writes()).toBe(0);
    await page.setOffline(false);
    await page.eventually(() => {
      expect(rules.text()).toContain("zip 最大 10 MB");
      expect(page.locator(".auth-name").text()).toBe("tester");
      expect(submit.isDisabled()).toBe(false);
    });
    expect(input.value()).toBe(draft);
    expect(writes()).toBe(0);
    await submit.click();
    await page.eventually(() =>
      expect(page.getByRole("status").filter({ hasText: "匯入完成" }).count()).toBe(1),
    );
    expect(writes()).toBe(1);
  } finally {
    await page.setOffline(false);
  }
});
