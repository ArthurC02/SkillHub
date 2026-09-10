import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import type { PreflightResponse } from "./api/lab";

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

const SKILL = "11111111-1111-1111-1111-111111111111";
const VERSION = "22222222-2222-2222-2222-222222222222";
const OLDER_VERSION = "44444444-4444-4444-4444-444444444444";
const TEST_CASE = "33333333-3333-3333-3333-333333333333";

const VERSIONS = {
  versions: [
    {
      version_id: VERSION,
      version_number: 2,
      content_hash: "sha256:bb",
      created_at: "2026-08-02T00:00:00Z",
    },
    {
      version_id: OLDER_VERSION,
      version_number: 1,
      content_hash: "sha256:aa",
      created_at: "2026-08-01T00:00:00Z",
    },
  ],
};

function summary(hash: string, files: string[]): PreflightResponse {
  return {
    summary_hash: hash,
    estimated_cost: {
      low_credits: 13,
      typical_credits: 78,
      high_credits: 390,
      basis: "估計值,非報價。來源:M2 基準試跑 45 個 Skill 的閘道實付分布。",
    },
    notes: ["以上任何一項變更都會產生新的摘要,必須重新確認。"],
    summary: {
      skill_version_id: VERSION,
      skill_content_hash: "sha256:abc",
      test_case_id: TEST_CASE,
      datasets: files.map((name, i) => ({
        dataset_id: `d${i}`,
        file_name: name,
        content_type: "text/plain",
        size_bytes: 1024,
        content_hash: `h${i}`,
      })),
      dataset_total_bytes: files.length * 1024,
      scripts: { status: "unavailable", findings: [] },
      tools: ["sandbox filesystem (/work, /out)"],
      mcp_servers: [],
      network: { mode: "default_deny", allow: [] },
      injected_secrets: ["ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"],
      provider: { name: "unassigned", rootless: false },
      resource_limits: {
        vcpu: 2,
        memory_bytes: 4 * 1024 ** 3,
        disk_bytes: 8 * 1024 ** 3,
        max_pids: 256,
        max_open_files: 1024,
        wall_clock_soft_seconds: 600,
        wall_clock_hard_seconds: 900,
        artifact_total_bytes: 100 << 20,
        artifact_file_bytes: 25 << 20,
        token_budget: { max_input_tokens: 300000, max_output_tokens: 60000 },
      },
    },
  };
}

function stubPlatform(
  initial: PreflightResponse = summary("hash-one", ["rows.csv"]),
  criteria: { id: string; text: string; source: "user"; confirmed_at: string | null }[] = [
    { id: "c1", text: "沒有重複的列", source: "user", confirmed_at: "2026-08-01T00:00:00Z" },
  ],
) {
  const calls: { url: string; body?: string }[] = [];
  let current = initial;
  let refusal = "";
  let runStatus = 0;
  let runBody: unknown = undefined;
  let ownSkillsStatus = 0;
  let testCaseStatus = 0;
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );

  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    calls.push({ url, body: init?.body as string | undefined });
    if (url.endsWith("/versions")) return json(VERSIONS);
    if (url.split("?")[0].endsWith("/skills")) {
      if (ownSkillsStatus) return json({ error: "own skills failed" }, ownSkillsStatus);
      return json({ skills: [{ skill_id: SKILL, name: "CSV 清理", summary: "整理 CSV。" }] });
    }
    if (url.includes(`/test-cases/${TEST_CASE}`)) {
      if (testCaseStatus) return json({ error: "test case failed" }, testCaseStatus);
      return json({
        test_case_id: TEST_CASE,
        skill_id: SKILL,
        name: "去重複列",
        user_prompt: "把重複的列去掉。",
        acceptance_criteria: criteria,
        created_at: "2026-08-01T00:00:00Z",
        updated_at: "2026-08-01T00:00:00Z",
      });
    }
    if (url.includes("/runs/preflight/confirm")) {
      const sent = JSON.parse(String(init?.body)) as { summary_hash: string };
      return sent.summary_hash === current.summary_hash
        ? json({ confirmed: true, summary_hash: sent.summary_hash }, 201)
        : json({ error: "summary_hash does not match the current permission summary" }, 422);
    }
    if (url.includes("/runs/preflight")) return json(current);
    if (url.includes("/runs")) {
      if (refusal) return json({ error: refusal }, 422);
      if (runStatus) return json(runBody ?? { error: "run failed" }, runStatus);
      const sent = JSON.parse(String(init?.body)) as { confirmed_summary_hash: string };
      return sent.confirmed_summary_hash === current.summary_hash
        ? json({ run_id: "run-1", status: "queued", provider: "unassigned" }, 201)
        : json({ error: "the permissions changed since it was confirmed" }, 422);
    }
    return json({ error: "not found" }, 404);
  });

  return {
    calls,
    changePermissions() {
      current = summary("hash-two", ["rows.csv", "extra.csv"]);
    },
    refuseWith(message: string) {
      refusal = message;
    },
    failOwnSkills(status: number) {
      ownSkillsStatus = status;
    },
    failTestCase(status: number) {
      testCaseStatus = status;
    },
    failRunWith(status: number, body?: unknown) {
      runStatus = status;
      runBody = body;
    },
  };
}

test("TEST-009 執行前確認要說出你正要跑的是哪一個 Skill、哪一段題目", async () => {
  stubPlatform();
  await renderLab();
  await waitFor(() => text().includes("去重複列"));

  expect(text()).toContain("CSV 清理");
  expect(text()).toContain("去重複列");
  expect(text()).not.toContain("不會產生逐條判定");
});

test("TEST-009 沒有驗收條件的 Run 會白跑，而這件事要在按下去之前說", async () => {
  stubPlatform(summary("hash-one", ["rows.csv"]), []);
  await renderLab();
  await waitFor(() => text().includes("去重複列"));

  expect(text()).toContain("不會產生逐條判定");
});

async function renderLab(
  search: {
    skill: string | undefined;
    version: string | undefined;
    test_case: string | undefined;
  } = {
    skill: SKILL,
    version: VERSION,
    test_case: TEST_CASE,
  },
) {
  const params = new URLSearchParams(
    Object.entries(search).filter(([, v]) => v !== undefined) as [string, string][],
  );
  window.history.pushState({}, "", `/lab/run?${params.toString()}`);
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    );
  });
  await act(async () => {
    await router.navigate({ to: "/lab/run", search });
  });
}

const text = () => container.textContent ?? "";

async function waitFor(done: () => boolean, timeoutMs = 2000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  throw new Error(`waitFor timed out; DOM was: ${container.textContent}`);
}

function versionSelect(): HTMLSelectElement {
  const select = container.querySelector<HTMLSelectElement>("select");
  if (!select) throw new Error(`no version picker; DOM was:\n${container.textContent}`);
  return select;
}

async function pickVersion(versionId: string) {
  const select = versionSelect();
  const setValue = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!;
  await act(async () => {
    setValue.call(select, versionId);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
}

function confirmButton(): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    b.textContent?.includes("開始 Run"),
  );
}

async function clickConfirm() {
  await waitFor(() => confirmButton() !== undefined);
  await act(async () => confirmButton()!.click());
}

test("02:TEST-005 the summary discloses every required item before the run starts", async () => {
  stubPlatform();
  await renderLab();

  const limits = container.textContent ?? "";
  if (!/vCPU\s*2(?!\s*秒)/.test(limits)) {
    throw new Error(`vCPU is not rendered as a bare count: ${limits}`);
  }
  if (!/時間上限\s*900\s*秒/.test(limits)) {
    throw new Error(`the hard wall clock is not rendered in seconds: ${limits}`);
  }

  const text = container.textContent ?? "";
  for (const label of [
    "Dataset",
    "Script",
    "工具",
    "MCP Server",
    "網路",
    "Secrets",
    "Provider",
    "資源上限",
  ]) {
    expect(text).toContain(label);
  }
  expect(text).toContain("rows.csv");
  expect(text).toContain("MCP Server");
  expect(text).toContain("無");
  expect(text).toContain("ANTHROPIC_AUTH_TOKEN");
  expect(text).not.toContain("sk-");
});

test("02:TEST-005 the fields inside summary_hash are all on screen, the quiet ones behind a disclosure", async () => {
  stubPlatform();
  await renderLab();

  const details = container.querySelector("details");
  expect(details, `no disclosure; DOM was:\n${container.textContent}`).not.toBeNull();
  const text = details?.textContent ?? "";
  expect(text).toContain("256");
  expect(text).toContain("1024");
  expect(text).toContain("100.0 MB");
  expect(text).toContain("25.0 MB");
  expect(text).toContain("600 秒");
  expect(text).toContain("rootless：否");
  expect(text).toContain("未測量");
});

test("PDM-005 §5.3 the pre-run screen shows an estimated cost range, labelled as an estimate", async () => {
  stubPlatform();
  await renderLab();

  const text = container.textContent ?? "";
  expect(text).toContain("預估點數");
  expect(text).toContain("估計值");
  expect(text).toContain("13 – 390 點");
  expect(text).not.toContain("US$");
  expect(text).not.toContain("$0.");
});

test("04 \u4e59-2 every resource ceiling is guarded, not only the four measured in bytes", async () => {
  const zeroed = summary("hash-zero", ["rows.csv"]);
  zeroed.summary.resource_limits = {
    vcpu: 0,
    memory_bytes: 0,
    disk_bytes: 0,
    max_pids: 0,
    max_open_files: 0,
    wall_clock_soft_seconds: 0,
    wall_clock_hard_seconds: 0,
    artifact_total_bytes: 0,
    artifact_file_bytes: 0,
    token_budget: { max_input_tokens: 0, max_output_tokens: 0 },
  };
  stubPlatform(zeroed);
  await renderLab();
  await waitFor(() => (container.textContent ?? "").includes("\u8cc7\u6e90\u4e0a\u9650"));

  const details = container.querySelector("details");
  if (details) details.open = true;

  const text = container.textContent ?? "";
  const refused = text.split("\u4f3a\u670d\u5668\u56de\u5831 0").length - 1;
  if (refused !== 12) {
    throw new Error(
      `${refused} of 12 ceiling renderings refused a zero; the rest printed it.\nDOM: ${text}`,
    );
  }
});

test("02:TEST-005 confirming sends the hash that was shown, then starts the run", async () => {
  const platform = stubPlatform();
  await renderLab();

  await clickConfirm();

  const confirm = platform.calls.find((c) => c.url.includes("/preflight/confirm"));
  expect(confirm?.body).toContain("hash-one");
  const started = platform.calls.find((c) => c.url.endsWith("/runs"));
  expect(started?.body).toContain("hash-one");
  expect(container.textContent).toContain("run-1");
});

test("02:TEST-005 a permission change forces a fresh confirmation instead of reusing the old one", async () => {
  const platform = stubPlatform();
  await renderLab();

  platform.changePermissions();

  await clickConfirm();
  expect(container.textContent).toContain("這次 Run 沒有開始");
  expect(container.textContent).toContain("summary_hash does not match");
  expect(platform.calls.some((c) => c.url.endsWith("/runs"))).toBe(false);
  expect(container.textContent).toContain("extra.csv");

  await clickConfirm();
  const sent = platform.calls.filter((c) => c.url.endsWith("/runs")).map((c) => c.body ?? "");
  expect(sent).toHaveLength(1);
  expect(sent[0]).toContain("hash-two");
  expect(container.textContent).toContain("run-1");
});

test("04 丙-14 the version comes from a picker, and a ?version= link is what it opens on", async () => {
  const platform = stubPlatform();
  await renderLab();

  expect(versionSelect().value).toBe(VERSION);
  expect(container.textContent).toContain("v2（最新）");
  expect(container.textContent).toContain("v1");

  await pickVersion(OLDER_VERSION);
  await waitFor(() =>
    platform.calls.some(
      (c) => c.url.includes("/runs/preflight") && c.url.includes(`version_id=${OLDER_VERSION}`),
    ),
  );
  expect(versionSelect().value).toBe(OLDER_VERSION);
});

test("04 丙-14 with no version in the URL the page asks for one instead of demanding an id", async () => {
  const platform = stubPlatform();
  await renderLab({ skill: SKILL, version: undefined, test_case: TEST_CASE });

  await waitFor(() => (container.textContent ?? "").includes("請先在上面選一個 Skill Version"));
  expect(platform.calls.some((c) => c.url.includes("version_id=&"))).toBe(false);

  await pickVersion(VERSION);
  await waitFor(() => (container.textContent ?? "").includes("資源上限"));
  expect(container.textContent).toContain("rows.csv");
});

test("SEC-002 gate B: an exhausted allowance is not reported as a permission change", async () => {
  const platform = stubPlatform();
  await renderLab();
  platform.refuseWith(
    "5 runs a day is the limit; it resets 24 hours after your earliest run today",
  );

  await clickConfirm();

  expect(container.textContent).toContain("這次 Run 沒有開始");
  expect(container.textContent).toContain("resets 24 hours after");
  expect(container.textContent).not.toContain("權限內容已變更");
});

test("02:RUN-003 the token ceiling says what it depends on, not just a number", async () => {
  stubPlatform();
  await renderLab();

  const text = container.textContent ?? "";
  expect(text).toContain("300000");
  expect(text).toContain("取決於每一輪的工具呼叫次數");
  expect(text).toContain("5 輪");
  expect(text).toContain("15 輪");
});

test("04 丙-148 ownSkills read failure says so, not 不在你的清單裡", async () => {
  const platform = stubPlatform();
  platform.failOwnSkills(500);
  await renderLab();

  await waitFor(() => text().includes("讀取失敗"));
  expect(text()).toContain("無法讀取你的 Skill 清單");
  expect(text()).not.toContain("不在你的清單裡");
});

test("04 丙-148 ownSkills read failure on 401 says login, not 讀取失敗 with a raw message", async () => {
  const platform = stubPlatform();
  platform.failOwnSkills(401);
  await renderLab();

  await waitFor(() => text().includes("需要登入"));
});

test("04 丙-148 testCase read failure says so, not 讀不到名稱", async () => {
  const platform = stubPlatform();
  platform.failTestCase(500);
  await renderLab();

  await waitFor(() => text().includes("讀取失敗"));
  expect(text()).toContain("無法讀取Test Case");
  expect(text()).not.toContain("讀不到名稱");
});

test("04 丙-144 the invite requirement is stated before the confirm button", async () => {
  stubPlatform();
  await renderLab();

  await waitFor(() => confirmButton() !== undefined);
  expect(text()).toContain("平台目前只讓有封測邀請的帳號開始 Run。");
});

test("04 丙-144 a 403 on run-start says no invite, not the raw server message", async () => {
  const platform = stubPlatform();
  platform.failRunWith(403, { error: "closed beta" });
  await renderLab();

  await clickConfirm();

  expect(text()).toContain("這個帳號還沒有封測邀請");
  expect(text()).not.toContain("closed beta");
});

test("04 丙-143 a 503 on run-start says try again, not the raw server message", async () => {
  const platform = stubPlatform();
  platform.failRunWith(503, { error: "sandbox pool temporarily unavailable" });
  await renderLab();

  await clickConfirm();

  expect(text()).toContain("再按一次");
  expect(text()).not.toContain("temporarily unavailable");
});

test("04 丙-143 a 404 on run-start says the skill version or test case is gone", async () => {
  const platform = stubPlatform();
  platform.failRunWith(404, { error: "run not found" });
  await renderLab();

  await clickConfirm();

  expect(text()).toContain("找不到");
  expect(text()).not.toContain("run not found");
});
