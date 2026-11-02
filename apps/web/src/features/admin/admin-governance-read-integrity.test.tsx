import { expect, test } from "vitest";
import { ADMIN_SKILLS } from "../../testing/fixtures/platform";
import { renderedApp } from "../../testing/app";

const app = renderedApp();

const SKILL = ADMIN_SKILLS.skills[0];
const BAD: [string, unknown][] = [
  ["null", null],
  ["string list", { skills: "" }],
  ["missing takedown", { skills: [{ ...SKILL, takedown_at: undefined }] }],
];

for (const phase of ["initial", "refresh"]) {
  for (const [name, malformed] of BAD) {
    test(`governance ${name} on ${phase} stays unknown and recovers`, async () => {
      const page = app();
      let body: unknown = phase === "initial" ? malformed : ADMIN_SKILLS;
      let reads = 0;
      const writes = () =>
        page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
      page.route("**/admin/skills?*", () => {
        reads++;
        return { json: body };
      });
      await page.goto(`/admin/skills?q=${SKILL.skill_id}`);
      const refresh = page.getByRole("button", { name: "重新整理治理狀態", exact: true });
      if (phase === "refresh") {
        await page.eventually(() =>
          expect(page.getByText("查到 1 筆小工具。", { exact: true }).count()).toBe(1),
        );
        body = malformed;
        await refresh.click();
      }
      const failure = page.getByRole("alert").filter({ hasText: "暫時無法讀取小工具" });
      await page.eventually(() => expect(failure.count()).toBeGreaterThan(0));
      expect(page.getByText("沒有符合", { exact: false }).count()).toBe(0);
      expect(page.getByText("治理狀態上次取得於", { exact: false }).count()).toBe(0);
      expect(page.getByRole("heading", { name: `對「${SKILL.name}」的動作` }).count()).toBe(0);
      expect(page.getByRole("status").filter({ hasText: "未下架" }).count()).toBe(0);
      expect(page.locator("#admin-skill-lift-takedown").count()).toBe(0);
      expect(await page.axeViolations("main")).toEqual([]);
      body = { skills: [] };
      await refresh.focus();
      await page.press("Enter");
      await page.eventually(() =>
        expect(
          page
            .getByText(`沒有符合「${SKILL.skill_id}」的小工具：0 筆。已清除的小工具不會出現。`, {
              exact: true,
            })
            .count(),
        ).toBe(1),
      );
      await page.eventually(() => expect(failure.count()).toBe(0));
      expect(reads).toBe(phase === "initial" ? 2 : 3);
      expect(writes()).toEqual([]);
    });
  }
}

test("governance receipt survives malformed reread", async () => {
  const page = app();
  let changed = false;
  let writes = 0;
  page.route(`**/admin/skills/${SKILL.skill_id}/redistribution`, () => {
    writes++;
    changed = true;
    return { json: { skill_id: SKILL.skill_id, redistribution: "blocked" } };
  });
  page.route("**/admin/skills?*", () => ({ json: changed ? { skills: "" } : ADMIN_SKILLS }));
  await page.goto(`/admin/skills?q=${SKILL.skill_id}`);
  await page.eventually(() =>
    expect(page.locator("#admin-skill-redistribution summary").count()).toBe(1),
  );
  await page.locator("#admin-skill-redistribution summary").click();
  await page.locator("#admin-redistribution-note").fill("禁止再散布");
  await page.locator("#admin-redistribution-value").select("blocked");
  await page.getByRole("button", { name: "送出判定", exact: true }).click();
  await page.eventually(() =>
    expect(page.getByRole("alert").text()).toContain("暫時無法讀取小工具"),
  );
  await page.eventually(() =>
    expect(page.locator("#admin-governance-result").text()).toBe(
      "「PDF Summariser」的再散布判定已改為「禁止再散布」。",
    ),
  );
  await page.eventually(() =>
    expect(page.locator("#admin-governance-result").isFocused()).toBe(true),
  );
  expect(page.locator("#admin-skill-redistribution").count()).toBe(0);
  expect(writes).toBe(1);
  expect(await page.axeViolations("main")).toEqual([]);
});
