import { expect, test, vi } from "vitest";
import {
  SKILL,
  TEST_CASE,
  VERSION,
  RUN,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { renderedApp } from "../../../testing/app";

const app = renderedApp();

const START_BUTTON = "我確認以上權限,開始試跑";

function respondRaw(page: ReturnType<typeof app>, endpoint: string, build: () => Response) {
  const platformFetch = globalThis.fetch;
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = new URL(String(input), "http://127.0.0.1");
    if (url.pathname !== endpoint || (init?.method ?? "GET").toUpperCase() !== "POST")
      return platformFetch(input, init);
    page.requests.push({ url, method: "POST", body: JSON.parse(String(init?.body)) });
    return Promise.resolve(build());
  });
}

function jsonBody(status: number, body: string) {
  return () => new Response(body, { status, headers: { "Content-Type": "application/json" } });
}

function truncatedBody() {
  const stream = new ReadableStream({
    start(controller) {
      controller.error(new TypeError("terminated"));
    },
  });
  return new Response(stream, { status: 201, headers: { "Content-Type": "application/json" } });
}

async function openPreflight(page: ReturnType<typeof app>) {
  await page.goto(`/skills/${SKILL}/test-cases/${TEST_CASE}/runs/new?version=${VERSION}`);
  await page.eventually(() => {
    expect(page.locator(".auth-name").text()).toBe("tester");
    expect(page.getByRole("button", { name: START_BUTTON, exact: true }).isDisabled()).toBe(false);
  });
}

const START_REPLIES: Record<string, () => Response> = {
  network: truncatedBody,
  server: jsonBody(503, JSON.stringify({ error: "service unavailable" })),
  malformed: jsonBody(201, "{"),
  incomplete: jsonBody(201, "{}"),
  null: jsonBody(201, "null"),
  valid: jsonBody(201, JSON.stringify({ run_id: RUN, status: "queued", provider: "unassigned" })),
};

for (const transport of Object.keys(START_REPLIES)) {
  test(`${transport} Run acknowledgement preserves an honest result without automatic replay`, async () => {
    const page = app();
    page.route("**/runs/preflight/confirm", () => ({
      json: { confirmed: true, summary_hash: "hash-one" },
    }));
    respondRaw(page, `/skills/${SKILL}/runs`, START_REPLIES[transport]);
    const confirms = () =>
      page.requests.filter((request) => request.url.pathname.endsWith("/runs/preflight/confirm"));
    const starts = () =>
      page.requests.filter((request) => request.url.pathname === `/skills/${SKILL}/runs`);
    await openPreflight(page);
    await page.getByRole("button", { name: START_BUTTON, exact: true }).click();
    const main = page.locator("main");
    if (transport === "valid") {
      const result = main.getByRole("link", { name: "查看這次試跑的結果", exact: true });
      await page.eventually(() => {
        expect(result.isFocused()).toBe(true);
        expect(result.attribute("href")).toBe(`/runs/${RUN}`);
        expect(main.getByRole("status").text()).toBe("已開始試跑。");
        expect(main.getByRole("alert").count()).toBe(0);
      });
    } else {
      const alert = main.getByRole("alert");
      await page.eventually(() => {
        expect(alert.text()).toBe(
          "無法確認試跑是否已開始。請先查看試跑紀錄，再決定是否重新送出；再次送出可能建立另一筆試跑並使用額度。",
        );
        expect(alert.isFocused()).toBe(true);
        expect(
          main.getByRole("button", { name: "核對後仍要再次開始試跑", exact: true }).isDisabled(),
        ).toBe(false);
        expect(main.getByText("已開始試跑。", { exact: true }).count()).toBe(0);
        expect(main.getByRole("link", { name: "查看這次試跑的結果", exact: true }).count()).toBe(0);
      });
    }
    expect(confirms()).toHaveLength(1);
    expect(starts()).toHaveLength(1);
    expect(starts()[0].body).toEqual({
      version_id: VERSION,
      test_case_id: TEST_CASE,
      confirmed_summary_hash: "hash-one",
    });
    expect(await page.axeViolations("body")).toEqual([]);
    if (transport !== "valid") {
      await main.getByRole("link", { name: "查看試跑紀錄", exact: true }).click();
      await expect(page.getByRole("heading", { name: "試跑活動", exact: true })).toHaveCount(1);
      expect(starts()).toHaveLength(1);
      expect(confirms()).toHaveLength(1);
    }
  });
}

for (const confirmation of ["server", "malformed"]) {
  test(`${confirmation} confirmation failure is known not to have sent Run creation`, async () => {
    const page = app();
    page.route(`**/skills/${SKILL}/runs`, ({ method }) =>
      method === "POST" ? { status: 500, json: { error: "unexpected start" } } : undefined,
    );
    respondRaw(
      page,
      `/skills/${SKILL}/runs/preflight/confirm`,
      confirmation === "server"
        ? jsonBody(503, JSON.stringify({ error: "service unavailable" }))
        : jsonBody(201, "{"),
    );
    const requestsTo = (suffix: string) =>
      page.requests.filter(
        (request) => request.method === "POST" && request.url.pathname.endsWith(suffix),
      );
    await openPreflight(page);
    await page.getByRole("button", { name: START_BUTTON, exact: true }).click();
    const main = page.locator("main");
    await page.eventually(() => {
      expect(main.getByRole("alert").text()).toBe(
        "權限確認未完成，尚未送出開始試跑要求。請重試權限確認。",
      );
      expect(main.getByRole("button", { name: START_BUTTON, exact: true }).isDisabled()).toBe(
        false,
      );
      expect(main.getByRole("link", { name: "查看試跑紀錄", exact: true }).count()).toBe(0);
    });
    expect(requestsTo("/runs/preflight/confirm")).toHaveLength(1);
    expect(requestsTo(`/skills/${SKILL}/runs`)).toHaveLength(0);
  });
}

for (const blockedSession of [false, true]) {
  test(`${blockedSession ? "session-blocked" : "refused"} Run start is not an uncertain creation`, async () => {
    const page = app();
    let checking = false;
    page.route("**/me", () =>
      checking
        ? { status: 503, json: { error: "service unavailable" } }
        : { json: platformResponse("/me").body },
    );
    page.route("**/runs/preflight/confirm", () => ({
      status: 403,
      json: { error: "closed beta" },
    }));
    const writes = () => page.requests.filter((request) => request.method === "POST");
    await openPreflight(page);
    if (blockedSession) {
      checking = true;
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
      await expect(page.getByRole("button", { name: "重新確認登入狀態", exact: true })).toHaveCount(
        1,
      );
    }
    await page.getByRole("button", { name: START_BUTTON, exact: true }).click();
    const main = page.locator("main");
    if (blockedSession) {
      await page.eventually(() => {
        expect(main.getByRole("alert").text()).toBe(
          "這次操作尚未送出。請先確認頁首的登入狀態，再自行送出。",
        );
        expect(main.getByRole("alert").isFocused()).toBe(true);
      });
    } else await expect(main.getByRole("alert")).toContainText("這個帳號還沒有封測邀請");
    await expect(main.getByRole("link", { name: "查看試跑紀錄", exact: true })).toHaveCount(0);
    await expect(
      main.getByRole("button", { name: "核對後仍要再次開始試跑", exact: true }),
    ).toHaveCount(0);
    expect(writes()).toHaveLength(blockedSession ? 0 : 1);
  });
}
