import { expect, test } from "vitest";
import { ADMIN_AGENTS, TEST_CASE } from "../../../testing/fixtures/platform";
import { renderedApp, type Locator } from "../../../testing/app";

const app = renderedApp();

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

function describedBy(field: Locator): string {
  const ids = (field.attribute("aria-describedby") ?? "").split(/\s+/).filter(Boolean);
  return ids
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .replace(/\s+/g, " ")
    .trim();
}

async function assertPhase(
  page: ReturnType<typeof app>,
  field: Locator,
  save: Locator,
  phase: Phase,
) {
  await field.fill(phase.value);
  await page.eventually(() => {
    expect(field.value()).toBe(phase.value);
    expect(save.isDisabled()).toBe(phase.invalid);
    if (phase.invalid) expect(field.attribute("aria-invalid")).toBe("true");
    else expect([null, "false"]).toContain(field.attribute("aria-invalid"));
    expect(describedBy(field)).toBe(phase.description);
    expect(field.isFocused()).toBe(true);
  });
}

const writes = (page: ReturnType<typeof app>) =>
  page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));

for (const width of [375, 1280]) {
  for (const scenario of capCases) {
    test(`Agent cap ${scenario.name} exposes errors and corrections at ${width}px`, async () => {
      const page = app();
      page.route("**/admin/agents", () => ({ json: { agents: [ADMIN_AGENTS.agents[0]] } }));
      await page.goto("/admin/agents");
      const summary = page.getByText("調整每日花費上限", { exact: true });
      await page.eventually(() => expect(summary.count()).toBe(1));
      await summary.click();
      const field = page.locator("#admin-agent-daily-report-cap");
      const note = page.locator("#admin-agent-daily-report-cap-set-note");
      const save = page.getByRole("button", { name: "改 daily-report 的每日上限", exact: true });
      await note.fill("檢查預算");
      await page.eventually(() => {
        expect(save.isDisabled()).toBe(false);
        expect([null, "false"]).toContain(field.attribute("aria-invalid"));
      });
      let inspectedInvalid = false;
      for (const phase of scenario.phases) {
        await assertPhase(page, field, save, phase);
        expect(note.value()).toBe("檢查預算");
        if (phase.invalid && !inspectedInvalid) {
          expect(await page.axeViolations("form:has(#admin-agent-daily-report-cap)")).toEqual([]);
          inspectedInvalid = true;
        }
      }
      expect(inspectedInvalid).toBe(true);
      expect(await page.axeViolations("form:has(#admin-agent-daily-report-cap)")).toEqual([]);
      expect(field.isFocused()).toBe(true);
      expect(writes(page)).toEqual([]);
    });
  }
  for (const scenario of factsCases) {
    test(`Simulated facts ${scenario.name} expose errors and corrections at ${width}px`, async () => {
      const page = app();
      await page.goto(`/lab/test-cases/${TEST_CASE}`);
      const field = page.getByLabel("使用者知道的事", { exact: true });
      const save = page.getByRole("button", { name: "儲存模擬使用者", exact: true });
      await page.eventually(() => expect(field.count()).toBe(1));
      await field.click();
      let inspectedInvalid = false;
      for (const phase of scenario.phases) {
        await assertPhase(page, field, save, phase);
        if (phase.invalid && !inspectedInvalid) {
          expect(page.locator("#simulated-user-limit").attribute("role")).toBe("status");
          expect(await page.axeViolations('section[aria-label="模擬使用者設計"]')).toEqual([]);
          inspectedInvalid = true;
        }
      }
      expect(inspectedInvalid).toBe(true);
      expect(await page.axeViolations('section[aria-label="模擬使用者設計"]')).toEqual([]);
      expect(field.isFocused()).toBe(true);
      expect(writes(page)).toEqual([]);
    });
  }
}
