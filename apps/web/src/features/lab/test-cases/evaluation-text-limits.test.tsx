import { expect, test } from "vitest";
import { renderedApp, type App, type Locator } from "../../../testing/app";
import { TEST_CASE } from "../../../testing/fixtures/platform";

const app = renderedApp();

const fields = [
  {
    name: "criterion",
    selector: "#criterion-c1",
    save: "儲存文字",
    cap: 2000,
    label: "驗收條件",
    scope: "li.criterion:has(#criterion-c1)",
  },
  {
    name: "new criterion",
    selector: "#new-criterion",
    save: "新增",
    cap: 2000,
    label: "驗收條件",
    scope: "p:has(#new-criterion)",
  },
  {
    name: "rubric item",
    selector: "#rubric-c1",
    save: "儲存 Rubric",
    cap: 2000,
    label: "Rubric 說明",
    scope: "li.criterion:has(#rubric-c1)",
  },
  {
    name: "rubric version",
    selector: "#rubric-version",
    save: "儲存 Rubric",
    cap: 200,
    label: "Rubric 版本",
    scope: "p:has(#rubric-version)",
  },
];

type Phase = { value: string; invalid: boolean; enabled: boolean; description: string | RegExp };

function writes(page: App) {
  return page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
}

test("Rubric version validation stays attached to the focused input through overflow, empty and correction", async () => {
  const page = app();
  await page.goto(`/lab/test-cases/${TEST_CASE}`);
  const input = page.locator("#rubric-version");
  await input.fill("X".repeat(201));
  const reason = page.locator("#rubric-version-reason");
  await expect(input).toHaveValue("X".repeat(201));
  await expect(reason).toHaveText("Rubric 版本最多 200 個字，目前 201 個字。");
  await expect(input).toBeFocused();
  await expect(reason).toBeVisible();
  expect(await page.axeViolations("p:has(#rubric-version)")).toEqual([]);
  await input.fill("");
  await expect(reason).toContainText("Rubric 版本是空的");
  await expect(input).toBeFocused();
  await expect(reason).toBeVisible();
  expect(await page.axeViolations("p:has(#rubric-version)")).toEqual([]);
  await input.fill("X".repeat(200));
  await expect(reason).toHaveCount(0);
  await expect(input).toBeFocused();
  await expect(page.getByRole("button", { name: "儲存 Rubric", exact: true })).toBeEnabled();
});

async function assertPhase(page: App, input: Locator, save: Locator, phase: Phase) {
  await input.fill(phase.value);
  await expect(input).toHaveValue(phase.value);
  if (phase.enabled) await expect(save).toBeEnabled();
  else await expect(save).toBeDisabled();
  if (phase.invalid) await expect(input).toHaveAttribute("aria-invalid", "true");
  else expect([null, "false"]).toContain(input.attribute("aria-invalid"));
  await expect(input).toHaveAccessibleDescription(phase.description);
  if (!phase.enabled) await expect(save).toHaveAccessibleDescription(phase.description);
  await expect(input).toBeFocused();
  if (phase.invalid) {
    const id = input.attribute("aria-describedby");
    expect(id).toBeTruthy();
    await expect(page.locator(`#${id}`)).toBeVisible();
  }
}

for (const field of fields) {
  for (const group of ["accepted boundaries", "invalid input and correction"]) {
    test(`Evaluation ${field.name} preserves native text and explains limits for ${group}`, async () => {
      const page = app();
      await page.goto(`/lab/test-cases/${TEST_CASE}`);
      const input = page.locator(field.selector);
      const owner =
        field.name === "criterion" ? page.locator("li.criterion").filter({ has: input }) : page;
      const save = owner.getByRole("button", { name: field.save, exact: true });
      if (field.name === "new criterion") {
        await expect(input).toHaveValue("");
        expect([null, "false"]).toContain(input.attribute("aria-invalid"));
        await expect(save).toBeDisabled();
        await expect(save).toHaveAccessibleDescription("欄位是空的");
      }
      const overflow = `${field.label}最多 ${field.cap} 個字，目前 ${field.cap + 1} 個字。`;
      const blank: Phase =
        field.name === "criterion"
          ? { value: "", invalid: true, enabled: false, description: "驗收條件不能是空白" }
          : field.name === "rubric version"
            ? { value: "", invalid: true, enabled: false, description: /Rubric 版本是空的/ }
            : field.name === "new criterion"
              ? { value: "", invalid: false, enabled: false, description: "欄位是空的" }
              : { value: "", invalid: false, enabled: true, description: "" };
      const boundaries: Phase[] = [
        { value: "X".repeat(field.cap), invalid: false, enabled: true, description: "" },
        { value: "𠮷".repeat(field.cap / 2 + 1), invalid: false, enabled: true, description: "" },
        { value: "𠮷".repeat(field.cap), invalid: false, enabled: true, description: "" },
      ];
      const recovery: Phase[] = [
        { value: "X".repeat(field.cap + 1), invalid: true, enabled: false, description: overflow },
        {
          value: "𠮷".repeat(field.cap + 1),
          invalid: true,
          enabled: false,
          description: overflow,
        },
        blank,
        { value: "𠮷".repeat(field.cap), invalid: false, enabled: true, description: "" },
      ];
      const phases = group === "accepted boundaries" ? boundaries : recovery;
      if (field.name === "rubric item" && group === "invalid input and correction") {
        phases.splice(phases.length - 1, 0, {
          value: " ".repeat(field.cap + 1),
          invalid: false,
          enabled: true,
          description: "",
        });
      }
      if (field.name === "rubric version") {
        phases.push({
          value: `  ${"X".repeat(field.cap)}  `,
          invalid: false,
          enabled: true,
          description: "",
        });
      }
      let inspectedInvalid = false;
      for (const phase of phases) {
        await assertPhase(page, input, save, phase);
        if (phase.invalid && !inspectedInvalid) {
          expect(await page.axeViolations(field.scope)).toEqual([]);
          inspectedInvalid = true;
        }
      }
      if (group === "invalid input and correction") {
        expect(inspectedInvalid).toBe(true);
        expect(await page.axeViolations(field.scope)).toEqual([]);
      }
      await expect(input).toBeFocused();
      expect(writes(page)).toEqual([]);
    });
  }
}
