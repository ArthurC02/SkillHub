import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { RUN, SUGGESTIONS } from "../../../testing/fixtures/platform";
import { renderedApp, type App } from "../../../testing/app";

const app = renderedApp();

const scrollIntoView = HTMLElement.prototype.scrollIntoView;

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
  if (scrollIntoView) HTMLElement.prototype.scrollIntoView = scrollIntoView;
  else delete (HTMLElement.prototype as Partial<HTMLElement>).scrollIntoView;
});

const DIFF = { target_path: "SKILL.md", applicable: true, unified_diff: "+ 清楚列出欄位" };
const DRAFT = "保留這份評估的補充說明";

async function inspectRead(page: App) {
  expect(await page.axeViolations("body")).toEqual([]);
}

for (const [kind, fault, payload] of [
  ["list", "null envelope", null],
  ["list", "missing items", { evaluation_id: SUGGESTIONS.evaluation_id }],
  [
    "list",
    "unknown choice",
    { ...SUGGESTIONS, suggestions: [{ ...SUGGESTIONS.suggestions[0], decision: "approve" }] },
  ],
  [
    "list",
    "unreadable evidence",
    { ...SUGGESTIONS, suggestions: [{ ...SUGGESTIONS.suggestions[0], evidence: [null] }] },
  ],
  ["diff", "null envelope", null],
  ["diff", "missing fields", {}],
  ["diff", "string applicability", { ...DIFF, applicable: "true" }],
  [
    "diff",
    "unknown reason",
    { target_path: "SKILL.md", applicable: false, blocked_reason: "unknown" },
  ],
] as const) {
  for (const phase of ["first read", "reread"] as const) {
    test(`${kind} with ${fault} on ${phase} keeps Run context and recovers only that section`, async () => {
      const page = app();
      let release!: () => void;
      const faultGate = new Promise<void>((resolve) => {
        release = resolve;
      });
      let reads = 0;
      const failAt = phase === "first read" ? 1 : 2;
      const writes: unknown[] = [];
      const endpoint = kind === "list" ? `**/runs/${RUN}/suggestions` : "**/suggestions/s1/diff";
      page.route(endpoint, async () => {
        reads++;
        if (reads === failAt) {
          await faultGate;
          return { json: payload };
        }
        return { json: kind === "list" ? SUGGESTIONS : DIFF };
      });
      page.route("**/suggestions/s1/decision", (request) => {
        writes.push(request.body);
        return { json: SUGGESTIONS.suggestions[0] };
      });
      try {
        await page.goto(`/runs/${RUN}`);
        await page.eventually(() => expect(page.locator(".auth-name").text()).toBe("tester"));
        const comment = page.locator("#feedback-comment");
        await page.eventually(() => expect(comment.count()).toBe(1));
        await comment.fill(DRAFT);
        if (kind === "list" && phase === "reread") {
          const accept = page
            .locator("li.suggestion")
            .getByRole("button", { name: "接受", exact: true });
          await page.eventually(() => expect(accept.count()).toBe(1));
          await accept.click();
        }
        const collapsed = page.getByRole("button", { name: "查看差異", exact: true });
        const expanded = page.getByRole("button", { name: "收起差異", exact: true });
        if (kind === "diff") {
          await page.eventually(() => expect(collapsed.attribute("aria-expanded")).toBe("false"));
          if (phase === "first read") await collapsed.press("Enter");
          else await collapsed.click();
          await page.eventually(() => expect(expanded.attribute("aria-expanded")).toBe("true"));
          if (phase === "reread") {
            await page.eventually(() =>
              expect(page.locator(".diff").text()).toContain("清楚列出欄位"),
            );
            await expanded.click();
            await page.eventually(() => {
              expect(collapsed.attribute("aria-expanded")).toBe("false");
              expect(collapsed.isFocused()).toBe(true);
              expect(page.getByRole("region", { name: "建議差異", exact: true }).count()).toBe(0);
            });
            await collapsed.press("Enter");
            await page.eventually(() => {
              expect(expanded.attribute("aria-expanded")).toBe("true");
              expect(expanded.isFocused()).toBe(true);
            });
          }
        }
        await page.eventually(() => expect(reads).toBe(failAt));
        release();
        const alert = page
          .getByRole("alert")
          .filter({ hasText: kind === "list" ? "暫時無法讀取改善建議" : "暫時無法讀取差異" });
        await page.eventually(() => {
          expect(alert.count()).toBe(1);
          expect(page.getByRole("heading", { name: "試跑結果", exact: true }).count()).toBe(1);
          expect(comment.value()).toBe(DRAFT);
          expect(
            page.getByRole("heading", { name: "這一頁暫時無法顯示", exact: true }).count(),
          ).toBe(0);
          expect(page.getByText("沒有可顯示的差異。", { exact: true }).count()).toBe(0);
          expect(page.locator(kind === "list" ? "li.suggestion" : ".diff").count()).toBe(0);
        });
        await inspectRead(page);
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
        if (kind === "list") {
          await page.eventually(() => {
            expect(page.getByRole("heading", { name: "改善建議", exact: true }).isFocused()).toBe(
              true,
            );
            expect(page.locator("li.suggestion").count()).toBe(1);
            expect(
              page
                .getByRole("button", { name: "以已接受的 1 項建議建立新版本", exact: true })
                .isDisabled(),
            ).toBe(false);
          });
        } else {
          await page.eventually(() => {
            expect(page.getByRole("region", { name: "建議差異", exact: true }).isFocused()).toBe(
              true,
            );
            expect(expanded.attribute("aria-expanded")).toBe("true");
            expect(page.locator(".diff").text()).toContain("清楚列出欄位");
          });
        }
        await page.eventually(() => expect(alert.count()).toBe(0));
        expect(comment.value()).toBe(DRAFT);
        expect(reads).toBe(failAt + 1);
        expect(writes).toEqual(
          kind === "list" && phase === "reread" ? [{ decision: "accepted" }] : [],
        );
      } finally {
        release();
      }
    });
  }
}

test("a readable empty improvement list stays distinct from a failed read", async () => {
  const page = app();
  page.route(`**/runs/${RUN}/suggestions`, () => ({
    json: { ...SUGGESTIONS, suggestions: [] },
  }));
  await page.goto(`/runs/${RUN}`);
  const message = page.getByText("這份評估沒有產生改善建議。", { exact: true });
  await page.eventually(() => expect(message.count()).toBe(1));
  expect(page.getByRole("alert").count()).toBe(0);
  expect(page.getByRole("button", { name: "再試一次", exact: true }).count()).toBe(0);
  await inspectRead(page);
});

for (const applicable of [true, false]) {
  test(`a readable empty diff with applicable=${applicable} keeps its actual meaning`, async () => {
    const page = app();
    page.route("**/suggestions/s1/diff", () => ({
      json: { target_path: "SKILL.md", applicable, unified_diff: "" },
    }));
    await page.goto(`/runs/${RUN}`);
    const open = page.getByRole("button", { name: "查看差異", exact: true });
    await page.eventually(() => expect(open.count()).toBe(1));
    await open.click();
    const region = page.getByRole("region", { name: "建議差異", exact: true });
    await page.eventually(() => {
      expect(region.text()).toContain("沒有可顯示的差異。");
      if (applicable) expect(region.text()).not.toContain("目前無法套用");
      else expect(region.text()).toContain("目前無法套用：伺服器沒有給原因。");
      expect(region.getByRole("alert").count()).toBe(0);
    });
    await inspectRead(page);
  });
}

for (const outcome of ["confirmed", "refused"] as const) {
  test(`a ${outcome} decision settles after another decision's list refresh unmounts their rows`, async () => {
    const page = app();
    let reads = 0;
    const items = ["s1", "s2"].map((id) => ({
      ...SUGGESTIONS.suggestions[0],
      suggestion_id: id,
      decision: "pending",
    }));
    page.route(`**/runs/${RUN}/suggestions`, () => {
      reads++;
      if (reads === 2) return { status: 503, json: { error: "controlled list outage" } };
      return { json: { ...SUGGESTIONS, suggestions: items } };
    });
    let release!: () => void;
    const secondGate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const writes: string[] = [];
    let secondDelivered = false;
    page.route("**/suggestions/*/decision", async (request) => {
      const id = request.url.pathname.split("/")[2];
      writes.push(id);
      if (id === "s2") {
        await secondGate;
        if (outcome === "refused") {
          secondDelivered = true;
          return { status: 400, json: { error: "controlled decision refusal" } };
        }
      }
      const item = items.find((candidate) => candidate.suggestion_id === id)!;
      item.decision = "accepted";
      if (id === "s2") secondDelivered = true;
      return { json: item };
    });
    try {
      await page.goto(`/runs/${RUN}`);
      await page.eventually(() => expect(page.locator(".auth-name").text()).toBe("tester"));
      const comment = page.locator("#feedback-comment");
      await page.eventually(() => expect(comment.count()).toBe(1));
      await comment.fill(DRAFT);
      const rows = page.locator("li.suggestion");
      await page.eventually(() => expect(rows.count()).toBe(2));
      await rows.nth(1).getByRole("button", { name: "接受", exact: true }).click();
      await page.eventually(() => expect(writes).toEqual(["s2"]));
      await rows.nth(0).getByRole("button", { name: "接受", exact: true }).click();
      await page.eventually(() => {
        expect(page.getByRole("alert").filter({ hasText: "暫時無法讀取改善建議" }).count()).toBe(1);
        expect(rows.count()).toBe(0);
      });
      release();
      await page.eventually(() => expect(secondDelivered).toBe(true));
      if (outcome === "refused")
        await page.getByRole("button", { name: "再試一次", exact: true }).click();
      await page.eventually(() => {
        expect(rows.count()).toBe(2);
        expect(
          page
            .getByRole("button", {
              name: `以已接受的 ${outcome === "confirmed" ? 2 : 1} 項建議建立新版本`,
              exact: true,
            })
            .isDisabled(),
        ).toBe(false);
        expect(page.locator("#suggestion-decisions-note").count()).toBe(0);
        expect(comment.value()).toBe(DRAFT);
      });
      expect(writes).toEqual(["s2", "s1"]);
      expect(reads).toBe(3);
      await inspectRead(page);
    } finally {
      release();
    }
  });
}
