import { expect, test } from "vitest";
import { renderedApp, type Locator } from "../../../testing/app";
import { TEST_CASE, VERSION } from "../../../testing/fixtures/platform";
import type { Dataset } from "../lab.service";

const app = renderedApp();

function chooseFile(input: Locator, name: string, content: string) {
  const element = input.element();
  const file = new File([content], name, { type: "text/csv" });
  let path = `C:\\fakepath\\${name}`;
  Object.defineProperty(element, "files", { value: [file], configurable: true });
  Object.defineProperty(element, "value", {
    get: () => path,
    set: (next: string) => {
      path = next;
      if (next === "") Object.defineProperty(element, "files", { value: [], configurable: true });
    },
    configurable: true,
  });
}

async function postedFileText(body: unknown): Promise<string> {
  return await ((body as FormData).get("file") as File).text();
}

for (const scenario of [
  { name: "clears its unchanged file selection", replacement: undefined },
  { name: "retains a differently named next file", replacement: "next.csv" },
  { name: "retains a same-name next file with new content", replacement: "report.csv" },
]) {
  test(`a completed dataset upload ${scenario.name}`, async () => {
    const page = app();
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: string[] = [];
    page.route(`**/test-cases/${TEST_CASE}/datasets`, async (request) => {
      if (request.method !== "POST") return undefined;
      writes.push(await postedFileText(request.body));
      const number = writes.length;
      if (number === 1) await gate;
      return {
        status: 201,
        json: {
          dataset_id: `00000000-0000-4000-8000-${String(number).padStart(12, "0")}`,
          file_name: number === 1 ? "report.csv" : (scenario.replacement ?? "report.csv"),
          size_bytes: number === 1 ? 10 : 11,
          content_type: "text/csv",
          content_hash: "a".repeat(64),
          expires_at: "2026-11-15T00:00:00Z",
        } satisfies Dataset,
      };
    });
    try {
      await page.goto(`/lab/test-cases/${TEST_CASE}/datasets?version=${VERSION}`);
      await expect(page.locator(".auth-name")).toHaveText("tester");
      const file = page.getByLabel("選擇檔案", { exact: true });
      const upload = page.getByRole("button", { name: "上傳", exact: true });
      await expect(upload).toBeEnabled();
      chooseFile(file, "report.csv", "first-data");
      await upload.click();
      await page.eventually(() => expect(writes).toHaveLength(1));
      await expect(page.getByRole("button", { name: "上傳中…", exact: true })).toBeDisabled();
      if (scenario.replacement) chooseFile(file, scenario.replacement, "second-data");
      release();
      const completed = page.getByRole("status", { name: "上傳完成", exact: true });
      await expect(completed).toContainText("已上傳 report.csv（10 B）");
      await expect(upload).toBeEnabled();
      expect(writes).toHaveLength(1);
      expect(writes[0]).toContain("first-data");
      if (scenario.replacement) {
        const kept = (file.element() as HTMLInputElement).files?.[0];
        expect({ name: kept?.name, size: kept?.size }).toEqual({
          name: scenario.replacement,
          size: 11,
        });
        expect(await page.axeViolations("body")).toEqual([]);
        expect(writes).toHaveLength(1);
        await upload.click();
        await expect(completed.locator("li")).toHaveCount(2);
        expect(writes).toHaveLength(2);
        expect(writes[1]).toContain("second-data");
      }
      await expect(file).toHaveValue("");
      expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
    } finally {
      release();
    }
  });
}
