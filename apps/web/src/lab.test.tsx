import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import App from "./App";
import { queryClient } from "./api/queryClient";
import { router } from "./router";
import type { PreflightResponse } from "./api/lab";

// 03:TEST-008/009 gate screen. Same hand-rolled DOM plumbing as disc.test.tsx —
// @testing-library is not a dependency of this app.

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

// Newest first, the order GET /skills/{id}/versions serves.
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
      currency: "USD",
      low: 0.01,
      typical: 0.06,
      high: 0.3,
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

/**
 * A fake platform whose permissions change under the page: the first summary
 * lists one file, and after `changePermissions()` it lists two and refuses the
 * old hash with 422 — which is exactly what the server does.
 */
function stubPlatform(
  initial: PreflightResponse = summary("hash-one", ["rows.csv"]),
  /** 驗收條件。空陣列是一個真實而且會讓整次 Run 白跑的狀態，所以它是一個參數。 */
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
    // 主詞的兩個來源。這一頁鉅細靡遺地列出這次 Run 碰得到什麼，卻沒說它是**誰**的
    // 什麼——所以這兩個端點以前在這個替身裡是 404，而沒有任何一支測試發現。
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
      // Gate B answers 422 for six different refusals, not one. `refuseWith`
      // lets a test be the server saying something other than "stale hash".
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
    /** 04 丙-148①②: ownSkills read fails with the given status (401/500/…). */
    failOwnSkills(status: number) {
      ownSkillsStatus = status;
    },
    /** 04 丙-148①②: testCaseInfo read fails with the given status. */
    failTestCase(status: number) {
      testCaseStatus = status;
    },
    /** 04 丙-143/144: the run-start hop itself answers non-422 (403/503/404/500). */
    failRunWith(status: number, body?: unknown) {
      runStatus = status;
      runBody = body;
    },
  };
}

/**
 * 那顆按鈕上寫著「我確認以上權限」，而在此之前這一頁從頭到尾沒說**你正在確認什麼**。
 *
 * 畫面給的是：一個只寫著 `v2（最新）・2026-08-02` 的版本下拉、一整份權限摘要、
 * 然後一顆按鈕。Skill 的名字沒有出現，Test Case 的名字沒有出現，會被逐條判定的
 * 驗收條件有幾條也沒有出現——唯一能認出主詞的線索是網址列裡的兩個 UUID。
 * `summary` 裡明明帶著 `test_case_id` 與 `skill_version_id`。
 *
 * 開兩個分頁、或從 RunCompare 按「以相同的 Test Case 與版本重新試跑」進來之後，
 * 讀者沒有任何辦法確認自己確認的是哪一次試跑。設計 §3 第 1、2 條。
 */
test("TEST-009 執行前確認要說出你正要跑的是哪一個 Skill、哪一段題目", async () => {
  stubPlatform();
  await renderLab();
  await waitFor(() => text().includes("去重複列"));

  expect(text()).toContain("CSV 清理");
  expect(text()).toContain("去重複列");
  // 有驗收條件的時候不該出現那句警告。
  expect(text()).not.toContain("不會產生逐條判定");
});

/**
 * 一個沒有驗收條件的 Test Case 跑得起來、會花掉額度、而且**不會產生任何判定**。
 * 在此之前沒有一個畫面提過這件事，包括最後一個可以反悔的這一個。
 */
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

/**
 * The button does not exist until the permission summary has arrived, and
 * renderLab() returns before it does. Every caller read the button straight
 * afterwards and the suite lost that race about one run in three (2026-08-24).
 * A flake is worse than an absent test: it teaches the next person to re-run
 * rather than to read.
 */
async function clickConfirm() {
  await waitFor(() => confirmButton() !== undefined);
  await act(async () => confirmButton()!.click());
}

test("02:TEST-005 the summary discloses every required item before the run starts", async () => {
  stubPlatform();
  await renderLab();

  // Units, not just numbers. The zeroed-ceiling test counts guards, and a
  // guard says nothing about which formatter it wraps: giving vCPU the
  // seconds formatter (「vCPU 2 秒」) left everything green (adversarial
  // review, 2026-08-24).
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
  // MVP has no MCP: shown as an explicit 無 rather than left out.
  expect(text).toContain("MCP Server");
  expect(text).toContain("無");
  // A secret is named, never valued.
  expect(text).toContain("ANTHROPIC_AUTH_TOKEN");
  expect(text).not.toContain("sk-");
});

// 設計 §1 原則 3: every field inside summary_hash has to be readable, or the
// user is re-confirming something they have never seen. The four resource
// limits and the provider details below the fold are in the hash, so they are
// on the page — collapsed, but present and reachable.
test("02:TEST-005 the fields inside summary_hash are all on screen, the quiet ones behind a disclosure", async () => {
  stubPlatform();
  await renderLab();

  const details = container.querySelector("details");
  expect(details, `no disclosure; DOM was:\n${container.textContent}`).not.toBeNull();
  const text = details?.textContent ?? "";
  expect(text).toContain("256"); // max_pids
  expect(text).toContain("1024"); // max_open_files
  expect(text).toContain("100.0 MB"); // artifact_total_bytes
  expect(text).toContain("25.0 MB"); // artifact_file_bytes
  expect(text).toContain("600 秒"); // wall_clock_soft_seconds
  // provider.rootless is false here and says so; runtime is absent and is
  // 未測量 (設計 §2.9 的表列詞) rather than an empty gap that reads as "none".
  expect(text).toContain("rootless：否");
  expect(text).toContain("未測量");
});

// PDM-005 §5.3/§5.2a-6. Both ends of the range have to be on screen, and the word
// "估計" has to be too — it sits outside summary_hash, so it must not read like
// one of the facts the user is agreeing to.
test("PDM-005 §5.3 the pre-run screen shows an estimated cost range, labelled as an estimate", async () => {
  stubPlatform();
  await renderLab();

  const text = container.textContent ?? "";
  expect(text).toContain("預估成本");
  expect(text).toContain("估計值");
  expect(text).toContain("$0.01");
  expect(text).toContain("$0.30");
});

test("04 \u4e59-2 every resource ceiling is guarded, not only the four measured in bytes", async () => {
  // A server reporting 0 for every ceiling. 0 is not a small limit; it is one no
  // run could pass, so it is a limit nobody is being held to. Printed bare it is
  // a number that satisfies every design-system rule sitting in front of somebody
  // about to press \u6211\u78ba\u8a8d \u2014 \u986f\u793a\u4f46\u4e0d\u5f37\u5236 (\u8a2d\u8a08 \u00a72.2, 04 \u4e59-2).
  //
  // Seven of the eleven fields were unguarded, because ceiling() ended in a call
  // to bytes() and so could only ever cover the byte-valued four (M2 audit,
  // 2026-08-24).
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

  // Open the disclosure: five of the twelve renderings live inside it, and a
  // number nobody can see is not what is being tested.
  const details = container.querySelector("details");
  if (details) details.open = true;

  // Twelve renderings of eleven fields \u2014 the hard wall clock is shown twice, in
  // the summary line and again beside the soft one.
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

  // The dataset changes after the page was rendered — the summary on screen is
  // now stale, and the confirmation built from it must not be accepted.
  platform.changePermissions();

  await clickConfirm();
  // The page says what it knows — nothing started — and hands over the
  // server's own sentence rather than inventing a cause. It used to report
  // every 422 as 「權限內容已變更」, which is a false statement for five of the
  // six refusals gate B answers with (M2 audit, 2026-08-24).
  expect(container.textContent).toContain("這次 Run 沒有開始");
  // The stale hash is caught one hop earlier than the run itself, so the
  // sentence on screen is the confirm endpoint's — which is the point: it is
  // the SERVER's, whichever hop refused.
  expect(container.textContent).toContain("summary_hash does not match");
  expect(platform.calls.some((c) => c.url.endsWith("/runs"))).toBe(false);
  // The page re-read the summary, so the second file is now on screen.
  expect(container.textContent).toContain("extra.csv");

  // Confirming the new summary sends the new hash, never the old one.
  await clickConfirm();
  const sent = platform.calls.filter((c) => c.url.endsWith("/runs")).map((c) => c.body ?? "");
  expect(sent).toHaveLength(1);
  expect(sent[0]).toContain("hash-two");
  expect(container.textContent).toContain("run-1");
});

// --- 04 丙-14: the version picker -------------------------------------------

test("04 丙-14 the version comes from a picker, and a ?version= link is what it opens on", async () => {
  const platform = stubPlatform();
  await renderLab();

  // The URL named a version, so that is the selection — not the first row of
  // the list, and not the newest. An existing link must keep meaning what it
  // said (there are two versions here, so "the default happens to be right"
  // cannot be what passes this).
  expect(versionSelect().value).toBe(VERSION);
  expect(container.textContent).toContain("v2（最新）");
  expect(container.textContent).toContain("v1");

  // Picking the older one re-reads the permission summary for that version:
  // 02:TEST-005's summary is per-version, so the screen must not keep showing
  // the previous one.
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

  // The old copy told the reader to supply a version id by hand; there is now a
  // list, and nothing is read until something is chosen.
  await waitFor(() => (container.textContent ?? "").includes("請先在上面選一個 Skill Version"));
  // Asserted as "never asked for an empty version" rather than "never asked":
  // the router is a module singleton these tests share, so a request left over
  // from another case's location proves nothing either way. What must not exist
  // is a preflight read for no version at all.
  expect(platform.calls.some((c) => c.url.includes("version_id=&"))).toBe(false);

  await pickVersion(VERSION);
  await waitFor(() => (container.textContent ?? "").includes("資源上限"));
  expect(container.textContent).toContain("rows.csv");
});

// The refusal a beta participant will actually meet, and the one the blanket
// message hid worst: an exhausted allowance carries the reset time precisely
// because "come back later" without a time is unactionable, and that sentence
// used to be thrown away and replaced with a claim about permissions.
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

/**
 * 02:RUN-003 / PDM-005 §5.2a-2: 「Token 上限必須連同輪數換算表一起呈現，不得只寫
 * 『300K』」.
 *
 * 02:TEST-005 records that this obligation had **three landing places and zero
 * implementations** — a search for 「輪」 or 「工具呼叫次數」 across `apps/web`,
 * `apps/platform` and `infra/images` returned nothing, while `03:TEST-011` and
 * `03:SBX-013` were both ticked. This is the permission-summary one.
 *
 * It is not formatting. `300000` is not readable, and the same 300K is ~5 rounds
 * for a tool-heavy run and ~15 for a conversational one — a factor of three, and
 * the only thing the reader is actually judging when they press 我確認.
 */
test("02:RUN-003 the token ceiling says what it depends on, not just a number", async () => {
  stubPlatform();
  await renderLab();

  const text = container.textContent ?? "";
  expect(text).toContain("300000");
  expect(text).toContain("取決於每一輪的工具呼叫次數");
  expect(text).toContain("5 輪");
  expect(text).toContain("15 輪");
});

// --- 04 丙-148①②: the shell header must not print absence for a failed read ---

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

// --- 04 丙-144: the invite gate is stated before the button, and on refusal ---

test("04 丙-144 the invite requirement is stated before the confirm button", async () => {
  stubPlatform();
  await renderLab();

  await waitFor(() => confirmButton() !== undefined);
  expect(text()).toContain("開始 Run 需要封測邀請，這道限制由平台強制");
});

test("04 丙-144 a 403 on run-start says no invite, not the raw server message", async () => {
  const platform = stubPlatform();
  platform.failRunWith(403, { error: "closed beta" });
  await renderLab();

  await clickConfirm();

  expect(text()).toContain("這個帳號還沒有封測邀請");
  expect(text()).not.toContain("closed beta");
});

// --- 04 丙-143(d): non-422 run-start failures get the page's own sentence ---

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
