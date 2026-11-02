import { afterAll, beforeAll, expect, test } from "vitest";
import { SKILL, SKILL_VERSIONS, skillDetail } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = () => {};
});
afterAll(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const COPIED = {
  skill_id: "abcdef01-2345-4789-abcd-0123456789ab",
  version_id: "abcdef02-2345-4789-abcd-0123456789ab",
  version_number: 1,
  name: "PDF Summariser-fork",
  summary: "",
  redistribution: "self_supplied",
};

const CREATE_FORK = "以這個小工具為起點建立我自己的";

for (const width of [375, 1280]) {
  test(`a long source name and fork receipt wrap without clipping at ${width}px`, async () => {
    const page = app();
    const name = "a".repeat(63);
    page.route(`**/api/skills/${SKILL}`, () => ({ json: skillDetail(SKILL, name) }));
    page.route(`**/skills/${SKILL}/fork`, () => ({
      status: 201,
      json: { ...COPIED, name: `${name}-fork` },
    }));
    await page.goto(`/skills/${SKILL}`);
    await page.eventually(() => {
      expect(page.locator(".auth-name").text()).toBe("tester");
      expect(page.getByRole("heading", { name, exact: true }).count()).toBe(1);
    });
    await page.getByRole("button", { name: CREATE_FORK, exact: true }).click();
    const receipt = page.getByRole("link", {
      name: `開啟 ${name}-fork v1 並繼續驗證`,
      exact: true,
    });
    await page.eventually(() => {
      expect(receipt.attribute("href")).toBe(
        `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
      );
      expect(receipt.isFocused()).toBe(true);
    });
    expect(await page.axeViolations("body")).toEqual([]);
  });
}

for (const [status, message] of [
  [400, "這次複製請求未獲接受"],
  [401, "複製小工具需要登入。"],
  [403, "這個帳號還沒有封測邀請"],
  [404, "這個小工具已經不在了"],
  [409, "副本已經有 10 份，名稱都用完了"],
] as const) {
  test(`HTTP ${status} refuses a fork without suggesting that its outcome is unknown`, async () => {
    const page = app();
    let writes = 0;
    page.route(`**/skills/${SKILL}/fork`, () => {
      writes += 1;
      return {
        status,
        json: {
          error:
            status === 403
              ? "Skill Hub is in closed beta: browsing and skill details are open to everyone, but forking, running and downloading are limited to invited participants."
              : status === 401
                ? "not authenticated"
                : status === 409
                  ? "你的工作區已經有同名的 Skill"
                  : "not found",
        },
      };
    });
    await page.goto(`/skills/${SKILL}`);
    await expect(page.locator(".auth-name")).toHaveText("tester");
    await page.getByRole("button", { name: CREATE_FORK, exact: true }).click();
    await expect(
      page.getByRole(status === 401 ? "status" : "alert").filter({ hasText: message }),
    ).toBeVisible();
    await expect(page.locator("main")).not.toContainText("尚未確認是否已複製");
    await expect(
      page.getByRole("link", { name: "另開資產庫確認複製結果", exact: true }),
    ).toHaveCount(0);
    if (status === 409)
      expect(
        page.getByRole("link", { name: "前往資產庫整理副本", exact: true }).attribute("href"),
      ).toBe("/library");
    expect(writes).toBe(1);
    expect(await page.axeViolations("body")).toEqual([]);
  });
}

test("forking another workspace's skill hands the keyboard off to the exact new owned version", async () => {
  const page = app();
  let writes = 0;
  page.route(`**/skills/${SKILL}/versions`, () => ({ json: { versions: [] } }));
  page.route(`**/skills/${SKILL}/fork`, () => {
    writes += 1;
    return { status: 201, json: COPIED };
  });
  page.route(`**/api/skills/${COPIED.skill_id}*`, () => ({
    json: {
      ...skillDetail(COPIED.skill_id, COPIED.name),
      version: {
        ...SKILL_VERSIONS.versions[0],
        version_id: COPIED.version_id,
        version_number: 1,
      },
    },
  }));
  page.route(`**/skills/${COPIED.skill_id}/versions`, () => ({
    json: {
      versions: [
        { ...SKILL_VERSIONS.versions[0], version_id: COPIED.version_id, version_number: 1 },
      ],
    },
  }));
  await page.goto(`/skills/${SKILL}`);
  await page.eventually(() => {
    expect(page.locator(".auth-name").text()).toBe("tester");
    expect(page.getByLabel("這個小工具是做什麼用的", { exact: true }).count()).toBe(0);
  });
  const create = page.getByRole("button", { name: CREATE_FORK, exact: true });
  expect(create.attribute("class")).toBe("action");
  await create.focus();
  await page.press("Enter");
  const receipt = page.getByRole("link", {
    name: "開啟 PDF Summariser-fork v1 並繼續驗證",
    exact: true,
  });
  await page.eventually(() => {
    expect(receipt.isFocused()).toBe(true);
    expect(receipt.attribute("href")).toBe(
      `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
    );
    expect(page.getByRole("button", { name: "已建立自己的版本", exact: true }).isDisabled()).toBe(
      true,
    );
  });
  expect(writes).toBe(1);
  await page.press("Enter");
  await page.eventually(() => {
    expect(page.getByRole("heading", { name: "PDF Summariser-fork v1", exact: true }).count()).toBe(
      1,
    );
    expect(page.locator(".version-facts").count()).toBe(1);
  });
  expect(writes).toBe(1);
  expect(await page.axeViolations("body")).toEqual([]);
});

for (const phase of ["confirmed", "pending"]) {
  test(`a ${phase} fork survives a source read failure without replaying or stealing draft focus`, async () => {
    const page = app();
    let copied = false;
    let changed = false;
    let writes = 0;
    let copiedBeforeCategory = false;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    page.route(`**/skills/${SKILL}/fork`, async () => {
      writes += 1;
      copied = true;
      if (phase === "pending") await gate;
      return { status: 201, json: COPIED };
    });
    page.route(`**/api/skills/${SKILL}`, () =>
      changed
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: skillDetail(SKILL, "PDF Summariser") },
    );
    page.route(`**/skills/${SKILL}/category`, () => {
      copiedBeforeCategory = copied;
      changed = true;
      return {
        json: { skill_id: SKILL, name: "PDF Summariser", summary: "", redistribution: "allowed" },
      };
    });
    const receipt = page.getByRole("link", {
      name: "開啟 PDF Summariser-fork v1 並繼續驗證",
      exact: true,
    });
    const choice = page.getByLabel("這個小工具是做什麼用的", { exact: true });
    try {
      await page.goto(`/skills/${SKILL}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      await page.getByRole("button", { name: CREATE_FORK, exact: true }).click();
      if (phase === "confirmed") {
        await page.eventually(() => {
          expect(receipt.attribute("href")).toBe(
            `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
          );
          expect(receipt.isFocused()).toBe(true);
        });
      } else {
        await page.eventually(() => {
          expect(page.getByRole("button", { name: "建立中…", exact: true }).isDisabled()).toBe(
            true,
          );
          expect(
            page.getByRole("status").filter({ hasText: "正在等待建立結果" }).count(),
          ).toBeGreaterThan(0);
        });
      }
      await page.eventually(() => expect(copied).toBe(true));
      await choice.select("writing");
      await page.getByRole("button", { name: "儲存", exact: true }).click();
      await expect(page.getByRole("heading", { name: "小工具總覽", exact: true })).toHaveCount(1);
      expect(copiedBeforeCategory).toBe(true);
      await choice.select("data");
      await choice.focus();
      release();
      await page.eventually(() => {
        expect(receipt.count()).toBe(1);
        expect(choice.isFocused()).toBe(true);
        expect(choice.value()).toBe("data");
        expect(
          page.getByRole("button", { name: "已建立自己的版本", exact: true }).isDisabled(),
        ).toBe(true);
      });
      expect(writes).toBe(1);
      expect(await page.axeViolations("body")).toEqual([]);
      changed = false;
      await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() => {
        expect(page.locator(".detail-answer").count()).toBe(1);
        expect(receipt.attribute("href")).toBe(
          `/skills/${COPIED.skill_id}/versions/${COPIED.version_id}`,
        );
        expect(choice.value()).toBe("data");
      });
      expect(writes).toBe(1);
      expect(await page.axeViolations("body")).toEqual([]);
    } finally {
      release();
    }
  });
}
