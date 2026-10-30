import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "../../../core/api/queryClient";
import { RunPreflight } from "./RunPreflight.page";
import {
  SKILL,
  RUN,
  TEST_CASE,
  VERSION,
  platformResponse,
} from "../../../testing/fixtures/platform";
import { pollUntil } from "../../../testing/poll";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  queryClient.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children }: { to: string; children?: unknown }) => (
    <a href={to}>{children as never}</a>
  ),
  useParams: () => ({ skillId: SKILL, runId: RUN, testCaseId: TEST_CASE }),
  useSearch: () => ({ skill: SKILL, version: VERSION, test_case: TEST_CASE }),
  useNavigate: () => () => Promise.resolve(),
}));

function platformWithPreflight(extra: Record<string, unknown>) {
  vi.stubGlobal("fetch", (input: string) => {
    const { body, status } = platformResponse(String(input));
    const path = String(input)
      .replace(/^https?:\/\/[^/]+/, "")
      .split("?")[0];
    const payload =
      path.endsWith("/runs/preflight") && body && typeof body === "object"
        ? { ...body, ...extra }
        : body;
    return Promise.resolve(
      new Response(JSON.stringify(payload), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

async function renderPreflight() {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <RunPreflight />
        </QueryClientProvider>
      </StrictMode>,
    );
  });
  await pollUntil(
    () =>
      queryClient.isFetching() === 0 && container.querySelector('[data-role="evidence"]') !== null,
    () => container.textContent,
    2000,
    { flushBeforeFirstCheck: true },
  );
  return container.textContent ?? "";
}

const startButton = () =>
  Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes("開始試跑"),
  );

test("a pair this deployment cannot run offers no button to start it", async () => {
  platformWithPreflight({ blocked: "content_not_curated" });
  const shown = await renderPreflight();

  expect(startButton(), "the refusal is known before the click, so the click must not exist").toBe(
    undefined,
  );
  expect(shown).toContain(
    "這個部署只跑目錄裡的小工具。這一版不在公開目錄、也不是被策展的那一版,所以按了也不會開始——要跑自己的小工具,請用有真正沙箱的部署。",
  );
});

test("an empty balance is named on the summary and tells the user who adds credits", async () => {
  platformWithPreflight({ blocked: "credit_balance" });
  const shown = await renderPreflight();

  expect(startButton(), "the run would be refused for credit, so there is nothing to click").toBe(
    undefined,
  );
  expect(shown).toContain(
    "點數不足,不能開始這次試跑。請聯絡管理者為這個帳號加點;加點後重新整理這一頁就能開始。",
  );
});

test("a balance short of the run's ceiling names the ceiling it was checked against", async () => {
  platformWithPreflight({ blocked: "credit_balance", ceiling_credits: 600 });
  const shown = await renderPreflight();

  expect(startButton()).toBe(undefined);
  expect(container.querySelector('[role="alert"].notice')?.textContent).toBe(
    "點數不夠這次試跑的上限 600 點，不能開始；實際只扣用掉的點數，但開始前餘額要夠付上限。請聯絡管理者為這個帳號加點；加點後重新整理這一頁就能開始。",
  );
  expect(shown).toContain("開始時會確認點數夠付這次試跑的上限 600 點；實際只扣用掉的點數。");
});

test.each([
  ["absent", undefined],
  ["zero", 0],
  ["negative", -1],
  ["fractional", 1.5],
  ["a string", "600"],
])("an unusable ceiling (%s) is not stated as a number", async (_, ceiling) => {
  platformWithPreflight({ blocked: "credit_balance", ceiling_credits: ceiling });
  const shown = await renderPreflight();

  expect(shown).not.toContain("的上限");
  expect(shown).toContain(
    "點數不足,不能開始這次試跑。請聯絡管理者為這個帳號加點;加點後重新整理這一頁就能開始。",
  );
});

test("a run the balance covers still says what the balance is checked against", async () => {
  platformWithPreflight({ ceiling_credits: 1 });
  const shown = await renderPreflight();

  expect(startButton()).toBeDefined();
  expect(shown).toContain("開始時會確認點數夠付這次試跑的上限 1 點；實際只扣用掉的點數。");
});

test("a platform budget that cannot cover the run names when it recovers and offers no button", async () => {
  platformWithPreflight({ blocked: "platform_model_budget" });
  const shown = await renderPreflight();

  expect(startButton(), "the run would be refused for the platform budget").toBe(undefined);
  expect(shown).toContain(
    "平台的模型總額度今天剩下的不夠這次試跑的上限,所以現在不能開始,也不會扣點。額度在台北時間每天上午 8 點恢復,之後重新整理這一頁就能開始。",
  );
});

for (const [blocked, says] of [
  ["workspace_concurrency", "這個工作區還有一次試跑沒有結束,一次只能跑一個"],
  ["quota_daily", "今天的試跑次數已經用完"],
  ["quota_window", "這段期間的試跑次數已經用完"],
  ["quota_unavailable", "現在算不出這個工作區還剩幾次試跑"],
] as const) {
  test(`a run the ${blocked} gate would refuse is named before confirming and offers no button`, async () => {
    platformWithPreflight({ blocked });
    const shown = await renderPreflight();

    expect(startButton(), `the run would be refused for ${blocked}`).toBe(undefined);
    expect(shown).toContain(says);
  });
}

test("a pair this deployment can run still offers the button", async () => {
  platformWithPreflight({});
  await renderPreflight();

  expect(startButton(), "nothing was blocked, so the run must still be startable").not.toBe(
    undefined,
  );
});

test("the safety gate separates run context from the permission evidence", async () => {
  platformWithPreflight({});
  await renderPreflight();

  const page = container.querySelector(".preflight-page");
  expect(page).not.toBeNull();
  expect(page!.querySelector(":scope > .preflight-context")).not.toBeNull();
  expect(page!.querySelector(":scope > .preflight-body [data-role='evidence']")).not.toBeNull();
});
