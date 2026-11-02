import { expect, test } from "vitest";
import { RUN, RUN_ARTIFACTS, platformResponse } from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

async function leaveAndReturn(page: ReturnType<typeof app>, change: () => void = () => {}) {
  await page.locator('a[href="/policy"]').first().click();
  await page.eventually(() => {
    expect(page.pathname()).toBe("/policy");
    expect(page.getByRole("heading", { name: "資料保存政策", exact: true }).count()).toBe(1);
  });
  change();
  await page.goBack();
}

for (const empty of [false, true]) {
  test(`artifact read failure withdraws cached ${empty ? "empty" : "populated"} claims and recovers locally`, async () => {
    const page = app();
    let reads = 0;
    page.route(`**/runs/${RUN}/artifacts`, () => {
      reads += 1;
      return reads === 2
        ? { status: 503, json: { error: "service unavailable" } }
        : {
            json: {
              ...RUN_ARTIFACTS,
              artifacts: empty && reads === 1 ? [] : RUN_ARTIFACTS.artifacts,
            },
          };
    });
    await page.goto(`/runs/${RUN}`);
    const section = page.locator("#run-artifacts");
    const items = section.locator(".download-item");
    const emptyNote = "這次試跑沒有留下任何檔案產出。";
    await page.eventually(() => {
      if (empty) expect(section.text()).toContain(emptyNote);
      else expect(items.count()).toBe(1);
    });
    await leaveAndReturn(page);
    await page.eventually(() => {
      expect(section.locator("[role=alert]").text()).toContain("產出清單");
      expect(items.count()).toBe(0);
      expect(section.text()).not.toContain(emptyNote);
    });
    await section.locator("button").filter({ hasText: "再試一次" }).click();
    await page.eventually(() => {
      expect(items.count()).toBe(1);
      expect(page.locator("#run-artifacts-heading").isFocused()).toBe(true);
    });
    expect(reads).toBe(3);
    expect(await page.axeViolations("main")).toEqual([]);
  });
}

test("run context failure hides cached source navigation and retries that owner alone", async () => {
  const page = app();
  let reads = 0;
  let failing = false;
  page.route(`**/runs/${RUN}`, ({ url }) => {
    reads += 1;
    return failing
      ? { status: 503, json: { error: "service unavailable" } }
      : { json: platformResponse(url.pathname + url.search).body };
  });
  await page.goto(`/runs/${RUN}`);
  const source = page.getByRole("navigation", { name: "這次試跑的來源", exact: true });
  await expect(source).toHaveCount(1);
  await leaveAndReturn(page, () => {
    failing = true;
  });
  const header = page.locator(".run-workspace-header");
  await page.eventually(() => {
    expect(header.locator("[role=alert]").text()).toContain("這次試跑");
    expect(source.count()).toBe(0);
  });
  const beforeRetry = reads;
  failing = false;
  await header.locator("button").filter({ hasText: "再試一次" }).click();
  await page.eventually(() => {
    expect(source.count()).toBe(1);
    expect(page.locator("#run-heading").isFocused()).toBe(true);
  });
  expect(reads).toBe(beforeRetry + 1);
});

for (const advanced of [false, true]) {
  test(`${advanced ? "advanced" : "general"} trace can retry a failed read without changing its URL context`, async () => {
    const page = app();
    let reads = 0;
    const offsets: string[] = [];
    const pattern = advanced ? `**/runs/${RUN}/trace?mode=advanced&*` : `**/runs/${RUN}/trace`;
    page.route(pattern, ({ url }) => {
      reads += 1;
      if (advanced) offsets.push(url.searchParams.get("after")!);
      return reads === 1
        ? { status: 403, json: { error: "read forbidden" } }
        : { json: platformResponse(url.pathname + url.search).body };
    });
    await page.goto(`/runs/${RUN}${advanced ? "?events=1000" : ""}`);
    const section = page.locator("#run-trace");
    const retry = section.locator("button").filter({ hasText: "再試一次" });
    await page.eventually(() => {
      expect(section.locator("[role=alert]").text()).toContain("無法讀取執行紀錄");
      expect(retry.count()).toBe(1);
    });
    await retry.click();
    await page.eventually(() => {
      expect(section.locator("[role=alert]").count()).toBe(0);
      expect(page.locator("#run-trace-heading").isFocused()).toBe(true);
      expect(reads).toBe(2);
    });
    if (advanced) expect(offsets).toEqual(["1000", "1000"]);
    else expect(new URLSearchParams(page.search()).get("events")).toBeNull();
  });
}

test("a failed live trace withdraws cached progress and cancellation until its state is readable", async () => {
  const page = app();
  let reads = 0;
  page.route(`**/runs/${RUN}/trace`, ({ url }) => {
    reads += 1;
    return reads === 2
      ? { status: 403, json: { error: "read forbidden" } }
      : {
          json: {
            ...(platformResponse(url.pathname + url.search).body as object),
            status: reads === 1 ? "running" : "succeeded",
          },
        };
  });
  await page.goto(`/runs/${RUN}`);
  const progress = page.getByText("進行中：執行中", { exact: true });
  const cancel = page.getByRole("button", { name: "取消這次試跑", exact: true });
  await page.eventually(() => {
    expect(progress.count()).toBe(1);
    expect(cancel.count()).toBe(1);
  });
  const section = page.locator("#run-trace");
  await page.eventually(() => {
    expect(section.locator("[role=alert]").text()).toContain("無法讀取執行紀錄");
    expect(progress.count()).toBe(0);
    expect(cancel.count()).toBe(0);
  });
  await section.locator("button").filter({ hasText: "再試一次" }).click();
  await page.eventually(() => {
    expect(section.locator("[role=alert]").count()).toBe(0);
    expect(section.text()).toContain("執行完成");
  });
  expect(reads).toBe(3);
});
