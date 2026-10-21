import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { ADMIN_AGENTS, TEST_CASE } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

type Phase = { value: string; invalid: boolean; description: string };

const CAP_REASON = "要填大於 0、最多 $5.0000 的金額，小數最多六位。";
const FACT_LIMIT = "最多 10 行，合計最多 1500 個字。";
const FACT_OVERFLOW = "目前合計 1501 個字，最多 1500 個字。";
const BOTH_LIMITS = "目前 11 行，最多 10 行。 目前合計 1501 個字，最多 1500 個字。";

const capCases: { name: string; phases: Phase[] }[] = [
  {
    name: "amount boundaries",
    phases: [
      { value: "0", invalid: true, description: CAP_REASON },
      { value: "0.000001", invalid: false, description: "" },
      { value: "5", invalid: false, description: "" },
      { value: "5.000001", invalid: true, description: CAP_REASON },
      { value: "5", invalid: false, description: "" },
    ],
  },
  {
    name: "precision",
    phases: [
      { value: "0.0000001", invalid: true, description: CAP_REASON },
      { value: "0.000001", invalid: false, description: "" },
    ],
  },
  {
    name: "malformed and empty input",
    phases: [
      { value: "abc", invalid: true, description: CAP_REASON },
      { value: "", invalid: true, description: CAP_REASON },
      { value: " 1 ", invalid: false, description: "" },
    ],
  },
];

const factsCases: { name: string; phases: Phase[] }[] = [
  {
    name: "plain boundary",
    phases: [
      { value: "字".repeat(1500), invalid: false, description: FACT_LIMIT },
      { value: "字".repeat(1501), invalid: true, description: FACT_OVERFLOW },
      { value: "字".repeat(1500), invalid: false, description: FACT_LIMIT },
    ],
  },
  {
    name: "astral boundary",
    phases: [
      { value: "𠮷".repeat(1500), invalid: false, description: FACT_LIMIT },
      { value: "𠮷".repeat(1501), invalid: true, description: FACT_OVERFLOW },
      { value: "𠮷".repeat(1500), invalid: false, description: FACT_LIMIT },
    ],
  },
  {
    name: "trimmed aggregate and optional clear",
    phases: [
      {
        value: `  ${"甲".repeat(750)}  \n\n ${"乙".repeat(750)}  `,
        invalid: false,
        description: FACT_LIMIT,
      },
      {
        value: `${"甲".repeat(750)}\n${"乙".repeat(751)}`,
        invalid: true,
        description: FACT_OVERFLOW,
      },
      { value: "  \n\n ", invalid: false, description: FACT_LIMIT },
    ],
  },
  {
    name: "both limits",
    phases: [
      {
        value: `${"字".repeat(1491)}\n${Array(10).fill("乙").join("\n")}`,
        invalid: true,
        description: BOTH_LIMITS,
      },
      { value: Array(10).fill("乙").join("\n"), invalid: false, description: FACT_LIMIT },
    ],
  },
];

function trackWrites(page: Page) {
  const writes: string[] = [];
  page.on("request", (request) => {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
  });
  return writes;
}

async function assertPhase(field: Locator, save: Locator, phase: Phase) {
  await field.fill(phase.value);
  await expect(field).toHaveValue(phase.value);
  if (phase.invalid) {
    await expect(save).toBeDisabled();
    await expect(field).toHaveAttribute("aria-invalid", "true");
  } else {
    await expect(save).toBeEnabled();
    expect([null, "false"]).toContain(await field.getAttribute("aria-invalid"));
  }
  await expect(field).toHaveAccessibleDescription(phase.description);
  await expect(field).toBeFocused();
  await expect(field).toBeInViewport({ ratio: 1 });
}

async function inspect(page: Page, scope: string, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include(scope).analyze();
  await writeFile(info.outputPath(`${state}-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}.png`) });
}

for (const width of [375, 1280]) {
  for (const scenario of capCases) {
    test(`Agent cap ${scenario.name} exposes errors and corrections at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      await page.route("**/admin/agents", (route) =>
        route.request().isNavigationRequest()
          ? route.fallback()
          : route.fulfill({ json: { agents: [ADMIN_AGENTS.agents[0]] } }),
      );
      const writes = trackWrites(page);
      await page.goto("/admin/agents");
      await page.getByText("調整每日花費上限", { exact: true }).click();
      const field = page.locator("#admin-agent-daily-report-cap");
      const note = page.locator("#admin-agent-daily-report-cap-set-note");
      const save = page.getByRole("button", { name: "改 daily-report 的每日上限", exact: true });
      await note.fill("檢查預算");
      await expect(save).toBeEnabled();
      expect([null, "false"]).toContain(await field.getAttribute("aria-invalid"));
      let inspectedInvalid = false;
      for (const phase of scenario.phases) {
        await assertPhase(field, save, phase);
        await expect(note).toHaveValue("檢查預算");
        if (phase.invalid && !inspectedInvalid) {
          await inspect(page, "form:has(#admin-agent-daily-report-cap)", info, "invalid");
          inspectedInvalid = true;
        }
      }
      expect(inspectedInvalid).toBe(true);
      await inspect(page, "form:has(#admin-agent-daily-report-cap)", info, "corrected");
      await expect(field).toBeFocused();
      expect(writes).toEqual([]);
    });
  }
  for (const scenario of factsCases) {
    test(`Simulated facts ${scenario.name} expose errors and corrections at ${width}px`, async ({
      page,
    }, info) => {
      await stubPlatform(page);
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
      const writes = trackWrites(page);
      await page.goto(`/lab/test-cases/${TEST_CASE}`);
      const field = page.getByLabel("使用者知道的事", { exact: true });
      const save = page.getByRole("button", { name: "儲存模擬使用者", exact: true });
      await field.click();
      let inspectedInvalid = false;
      for (const phase of scenario.phases) {
        await assertPhase(field, save, phase);
        if (phase.invalid && !inspectedInvalid) {
          await expect(page.locator("#simulated-user-limit")).toHaveAttribute("role", "status");
          await inspect(page, 'section[aria-label="模擬使用者設計"]', info, "invalid");
          inspectedInvalid = true;
        }
      }
      expect(inspectedInvalid).toBe(true);
      await inspect(page, 'section[aria-label="模擬使用者設計"]', info, "corrected");
      await expect(field).toBeFocused();
      expect(writes).toEqual([]);
    });
  }
}
