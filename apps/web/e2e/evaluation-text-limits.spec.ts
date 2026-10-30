import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page, type TestInfo } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { TEST_CASE } from "../src/testing/fixtures/platform";
import { stubPlatform } from "./stub";

test.describe.configure({ mode: "default" });

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

for (const width of [375, 1280]) {
  test(`Rubric version validation remains visible when typing near the viewport bottom at ${width}px`, async ({
    page,
  }, info) => {
    await stubPlatform(page);
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/lab/test-cases/${TEST_CASE}`);
    const input = page.locator("#rubric-version");
    await input.click();
    await input.press("ControlOrMeta+A");
    await input.evaluate((element) =>
      window.scrollTo({
        top: scrollY + element.getBoundingClientRect().bottom - innerHeight + 16,
        behavior: "instant",
      }),
    );
    await page.keyboard.insertText("X".repeat(201));
    const reason = page.locator("#rubric-version-reason");
    await expect(input).toHaveValue("X".repeat(201));
    await expect(reason).toHaveText("Rubric 版本最多 200 個字，目前 201 個字。");
    await expect(input).toBeFocused();
    await expect(input).toBeInViewport({ ratio: 1 });
    await expect(reason).toBeInViewport({ ratio: 1 });
    await inspect(page, "p:has(#rubric-version)", info, "bottom-overflow");
    await replaceText(page, input, "");
    await expect(reason).toContainText("Rubric 版本是空的");
    await expect(input).toBeFocused();
    await expect(input).toBeInViewport({ ratio: 1 });
    await expect(reason).toBeInViewport({ ratio: 1 });
    await inspect(page, "p:has(#rubric-version)", info, "bottom-empty");
    await replaceText(page, input, "X".repeat(200));
    await expect(reason).toHaveCount(0);
    await expect(input).toBeFocused();
    await expect(page.getByRole("button", { name: "儲存 Rubric", exact: true })).toBeEnabled();
  });
}

async function replaceText(page: Page, input: Locator, value: string) {
  await input.click();
  await input.press("ControlOrMeta+A");
  if (value === "") await input.press("Backspace");
  else await page.keyboard.insertText(value);
}

async function assertPhase(page: Page, input: Locator, save: Locator, phase: Phase) {
  await replaceText(page, input, phase.value);
  await expect(input).toHaveValue(phase.value);
  if (phase.enabled) await expect(save).toBeEnabled();
  else await expect(save).toBeDisabled();
  if (phase.invalid) await expect(input).toHaveAttribute("aria-invalid", "true");
  else expect([null, "false"]).toContain(await input.getAttribute("aria-invalid"));
  await expect(input).toHaveAccessibleDescription(phase.description);
  if (!phase.enabled) await expect(save).toHaveAccessibleDescription(phase.description);
  await expect(input).toBeFocused();
  await expect(input).toBeInViewport({ ratio: 1 });
  if (phase.invalid) {
    const id = await input.getAttribute("aria-describedby");
    expect(id).toBeTruthy();
    await expect(page.locator(`#${id}`)).toBeInViewport({ ratio: 1 });
  }
}

async function inspect(page: Page, scope: string, info: TestInfo, state: string) {
  const report = await new AxeBuilder({ page }).include(scope).analyze();
  await writeFile(info.outputPath(`${state}-limits-a11y.json`), JSON.stringify(report));
  expect(report.violations).toEqual([]);
  expect(report.incomplete).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath(`${state}.png`) });
}

for (const field of fields) {
  for (const width of [375, 1280]) {
    for (const group of ["accepted boundaries", "invalid input and correction"]) {
      test(`Evaluation ${field.name} preserves native text and explains limits for ${group} at ${width}px`, async ({
        page,
      }, info) => {
        await stubPlatform(page);
        await page.setViewportSize({ width, height: 900 });
        await page.emulateMedia({ colorScheme: width === 375 ? "light" : "dark" });
        const writes: string[] = [];
        page.on("request", (request) => {
          if (!["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(request.url());
        });
        await page.goto(`/lab/test-cases/${TEST_CASE}`);
        const input = page.locator(field.selector);
        const owner =
          field.name === "criterion" ? page.locator("li.criterion").filter({ has: input }) : page;
        const save = owner.getByRole("button", { name: field.save, exact: true });
        if (field.name === "new criterion") {
          await expect(input).toHaveValue("");
          expect([null, "false"]).toContain(await input.getAttribute("aria-invalid"));
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
          {
            value: "X".repeat(field.cap + 1),
            invalid: true,
            enabled: false,
            description: overflow,
          },
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
        await input.evaluate((element) => element.scrollIntoView({ block: "end" }));
        let inspectedInvalid = false;
        for (const phase of phases) {
          await assertPhase(page, input, save, phase);
          if (phase.invalid && !inspectedInvalid) {
            await inspect(page, field.scope, info, "invalid");
            inspectedInvalid = true;
          }
        }
        if (group === "invalid input and correction") {
          expect(inspectedInvalid).toBe(true);
          await inspect(page, field.scope, info, "corrected");
        }
        await expect(input).toBeFocused();
        expect(writes).toEqual([]);
      });
    }
  }
}
