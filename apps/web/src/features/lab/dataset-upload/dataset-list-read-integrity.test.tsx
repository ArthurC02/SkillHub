import { expect, test } from "vitest";
import { renderedApp, type Locator } from "../../../testing/app";
import { TEST_CASE, VERSION } from "../../../testing/fixtures/platform";

const app = renderedApp();

const ROW = {
  dataset_id: "11111111-1111-4111-8111-111111111111",
  file_name: "existing.csv",
  content_type: "text/csv",
  content_hash: "a".repeat(64),
  size_bytes: 1000,
  expires_at: "2026-11-15T00:00:00Z",
};
const MALFORMED: [string, unknown][] = [
  ["string files", { datasets: "", total_bytes: 0 }],
  ["missing file expiry", { datasets: [{ ...ROW, expires_at: undefined }], total_bytes: 1000 }],
  ["inconsistent total", { datasets: [ROW], total_bytes: 0 }],
];

function chooseFile(input: Locator, name: string, content: string) {
  const element = input.element();
  const file = new File([content], name, { type: "text/csv" });
  Object.defineProperty(element, "files", { value: [file], configurable: true });
  Object.defineProperty(element, "value", {
    value: `C:\\fakepath\\${name}`,
    writable: true,
    configurable: true,
  });
}

for (const [owner, phase] of [
  ["detail", "initial"],
  ["detail", "cached"],
  ["upload", "initial"],
  ["upload", "cached"],
]) {
  for (const [name, malformed] of MALFORMED) {
    test(`dataset ${owner} with ${name} on ${phase} withholds unsupported decisions and recovers`, async () => {
      const page = app();
      let current: unknown =
        phase === "initial" ? malformed : { datasets: [ROW], total_bytes: 1000 };
      let reads = 0;
      const errors: string[] = [];
      const onError = (event: ErrorEvent) => errors.push(event.message);
      window.addEventListener("error", onError);
      const writes = () =>
        page.requests.filter((request) => !["GET", "HEAD", "OPTIONS"].includes(request.method));
      page.route(`**/test-cases/${TEST_CASE}/datasets`, () => {
        reads++;
        return { status: 200, body: JSON.stringify(current) };
      });
      try {
        const path = `/lab/test-cases/${TEST_CASE}${owner === "upload" ? "/datasets" : ""}?version=${VERSION}`;
        await page.goto(path);
        const upload = page.getByRole("button", { name: "上傳", exact: true });
        if (phase === "cached") {
          if (owner === "upload") await expect(upload).toBeEnabled();
          else await expect(page.getByText("existing.csv", { exact: false })).toBeVisible();
          await page.locator('a[href="/policy"]').first().click();
          await expect(
            page.getByRole("heading", { name: "資料保存政策", exact: true }),
          ).toBeVisible();
          current = malformed;
          await page.goBack();
        }
        await page.eventually(() => expect(reads).toBe(phase === "initial" ? 1 : 2));
        const failure = page.getByRole("alert").filter({
          hasText: owner === "upload" ? "暫時無法讀取測試資料已用容量" : "暫時無法讀取檔案清單",
        });
        await expect(failure).toBeVisible();
        await expect(page.getByText("還沒有上傳任何檔案。", { exact: true })).toHaveCount(0);
        await expect(page.getByText("existing.csv", { exact: false })).toHaveCount(0);
        await expect(page.getByRole("button", { name: "刪除這個檔案", exact: true })).toHaveCount(
          0,
        );
        if (owner === "upload") {
          await expect(upload).toBeDisabled();
          await expect(page.locator("#upload-read-why")).toContainText("目前無法確認");
          await expect(page.getByText("還可以再上傳", { exact: false })).toHaveCount(0);
          chooseFile(page.getByLabel("選擇檔案", { exact: true }), "chosen.csv", "n".repeat(1000));
        }
        expect(await page.axeViolations("main")).toEqual([]);
        current = { datasets: [], total_bytes: 0 };
        const retry = page.getByRole("button", { name: "再試一次", exact: true });
        await retry.focus();
        await expect(retry).toBeFocused();
        await retry.press("Enter");
        await expect(failure).toHaveCount(0);
        if (owner === "upload") {
          await expect(upload).toBeEnabled();
          await expect(page.getByLabel("選擇檔案", { exact: true })).toHaveValue(/chosen\.csv$/);
          await expect(page.locator("#dataset-heading")).toBeFocused();
        } else {
          await expect(page.getByText("還沒有上傳任何檔案。", { exact: true })).toBeVisible();
          await expect(page.locator("#test-case-datasets-heading")).toBeFocused();
        }
        expect(reads).toBe(phase === "initial" ? 2 : 3);
        expect(writes()).toEqual([]);
        expect(errors).toEqual([]);
        expect(new URLSearchParams(page.search()).get("version")).toBe(VERSION);
      } finally {
        window.removeEventListener("error", onError);
      }
    });
  }
}
