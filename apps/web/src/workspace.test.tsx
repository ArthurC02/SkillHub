import { StrictMode, act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { queryClient } from "./api/queryClient";
import { Downloads } from "./pages/Downloads";
import { RunTrace } from "./pages/RunTrace";
import { WorkspaceAccount } from "./pages/WorkspaceAccount";
import { WorkspaceRuns } from "./pages/WorkspaceRuns";
import { WorkspaceSkills } from "./pages/WorkspaceSkills";
import { useForkSkill } from "./api/skills";

// 02:WS-002 第 1 條 / WS-004 — the workspace's own lists, plus 第 3 條's delete
// on what a run produced (02:SEC-006). Same hand-rolled DOM plumbing as the
// other suites; @testing-library is not a dependency of this app.

const SKILL = "11111111-1111-1111-1111-111111111111";
const RUN = "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20";
const ARTIFACT = "33333333-3333-3333-3333-333333333333";

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
  Link: ({
    to,
    params,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => (
    <a href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}>
      {children as never}
    </a>
  ),
  useParams: () => ({ runId: RUN }),
  useSearch: () => ({}),
  // /runs/$runId carries `evaluation` and `events` now (資訊架構 §0.1 R4); this
  // file renders that page for the artifact rows and never navigates.
  useNavigate: () => () => Promise.resolve(),
}));

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

async function render(node: ReactNode, settled: () => boolean) {
  await act(async () => {
    root = createRoot(container);
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
      </StrictMode>,
    );
  });
  await waitFor(settled);
}

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

function button(text: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find((b) =>
    (b.textContent ?? "").includes(text),
  );
}

const text = () => container.textContent ?? "";

/** Opens the disclosure whose summary says `label`, the way a click would. */
async function open(label: string) {
  const details = Array.from(container.querySelectorAll("details")).find((d) =>
    (d.querySelector("summary")?.textContent ?? "").includes(label),
  )!;
  await act(async () => {
    details.open = true;
    details.dispatchEvent(new Event("toggle"));
  });
}

// --- the run history --------------------------------------------------------

const RUN_ROW = {
  run_id: RUN,
  status: "succeeded",
  skill_id: SKILL,
  skill_name: "CSV 清理",
  skill_version_id: "22222222-2222-2222-2222-222222222222",
  provider: "self-hosted",
  cleanup_status: { value: "cleaned", label: "已清理", note: "沙箱與其資源已回收。" },
  // 04 丙-32: the second axis. Required and never null — 未評估 is a value, not an
  // omission, because an empty verdict beside 「執行完成」 reads as a pass.
  evaluation: {
    value: "met",
    label: "符合",
    note: "依這個 Run 當時的驗收條件判定為符合。",
  },
  created_at: "2026-08-17T00:00:00Z",
  finished_at: "2026-08-17T00:04:00Z",
};

test("WS-004 a run history row words `succeeded` as execution, never as a pass", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [RUN_ROW] }));
  await render(<WorkspaceRuns />, () => text().includes("CSV 清理"));

  // ADR-025: a list is where "the workload finished" is cheapest to misread as
  // "the task was done", so the row says which one it means.
  expect(text()).toContain("執行狀態：執行完成");
  // ADR-025 / 設計 §2.5: two axes, verdict first. This used to assert the
  // apology — a footnote saying the one axis on screen was not the other one —
  // because RunListItem had no verdict field until 04 丙-32. Now it asserts the
  // second axis is really there and really ahead of the first.
  expect(text()).toContain("任務判定：符合");
  expect(text().indexOf("任務判定")).toBeLessThan(text().indexOf("執行狀態"));
  expect(text()).not.toContain("成功");
});

test("WS-004 an unevaluated run says 未評估, which is not a blank and not a pass", async () => {
  // 04 丙-32 / §2.9. The whole reason `evaluation` is required rather than
  // nullable: an empty verdict column beside 「執行完成」 reads as a pass, and this
  // is the commonest row in any history — most runs are never evaluated.
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        {
          ...RUN_ROW,
          evaluation: {
            value: "not_evaluated",
            label: "未評估",
            note: "這個 Run 還沒有任務判定。執行狀態說的是工作負載跑完了沒有,不是任務有沒有做到(ADR-025)。",
          },
        },
        {
          ...RUN_ROW,
          run_id: "run-9",
          evaluation: {
            value: "evaluation_failed",
            label: "評估失敗",
            note: "判定沒有產生出來。這不代表任務失敗——沒有人判過,不是判過不合格。",
          },
        },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("CSV 清理"));

  expect(text()).toContain("任務判定：未評估");
  // The note rides along for exactly the states whose label can be misread; the
  // three real verdicts speak for themselves and would be fifty repetitions of
  // one sentence on a full page (04 丙-29 裁定① rejected that shape for status).
  expect(text()).toContain("不是任務有沒有做到");
  // 「評估失敗」 is the one most likely to be read as a task failure, so it says
  // it is not — and it must not be tinted as a failure either.
  expect(text()).toContain("任務判定：評估失敗");
  expect(text()).toContain("這不代表任務失敗");
  expect(container.querySelectorAll(".badge-danger")).toHaveLength(0);
});

test("WS-004 a run whose sandbox was not cleaned up says so on the row", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        { ...RUN_ROW, status: "failed", status_reason: "the provider could not carry the attempt" },
        {
          ...RUN_ROW,
          run_id: "run-2",
          cleanup_status: { value: "failed", label: "清理失敗", note: "平台會重試。" },
        },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("執行失敗"));

  expect(text()).toContain("the provider could not carry the attempt");
  expect(text()).toContain("清理失敗");
  // 設計系統 §2.1: 已清理 is a fact the owner wants — the sandbox was torn down —
  // and rendering nothing for it made the majority case indistinguishable from a
  // field that was never rendered at all.
  expect(text()).toContain("已清理");
  // §4.4: --accent-border is 未知／未驗證 and --danger is 這件事不通過. 清理失敗 is
  // the second, so it must not wear the class 尚未清理 and 清理中 wear.
  expect(container.querySelectorAll(".badge-unverified")).toHaveLength(0);
  expect(container.querySelectorAll(".badge-danger")).toHaveLength(1);
});

test("WS-004 a cleanup state the client does not recognise is still rendered as words", async () => {
  // The union in api/runs.ts said `cleaning` where the database enum
  // run_cleanup_status says `cleaning_up` (0004_test_lab_and_runs.sql:75), and
  // trial/execution/http.go:291 puts the database value on the wire unmapped.
  // So the one state this row exists to report — the sandbox is being torn down
  // right now — arrived as a key no map had, and 設計系統 §2.9 is explicit that a
  // blank is the one shape an absence may never take. Being total over the union
  // could not catch it: the union was the thing that was wrong.
  vi.stubGlobal("fetch", () =>
    json({
      runs: [
        {
          ...RUN_ROW,
          cleanup_status: { value: "cleaning_up", label: "清理中", note: "會自己結束。" },
        },
        {
          ...RUN_ROW,
          run_id: "run-3",
          // The server keeps the raw value as its own label when it has no words
          // for it, so this build renders something rather than a blank.
          cleanup_status: {
            value: "a-state-from-a-newer-server",
            label: "a-state-from-a-newer-server",
            note: "這個平台版本沒有這個清理狀態的說明,值照原樣顯示,不猜測它的意思。",
          },
        },
      ],
    }),
  );
  await render(<WorkspaceRuns />, () => text().includes("清理中"));

  expect(text()).toContain("清理狀態：清理中");
  // Not a translation — the platform said something this build has no word for,
  // and saying so is honest where a blank would read as "nothing to report".
  expect(text()).toContain("a-state-from-a-newer-server");
  for (const badge of container.querySelectorAll(".badge")) {
    expect(badge.textContent?.trim(), "a badge with no word (§2.3)").not.toBe("");
  }
});

test("WS-002 an empty run history says nothing ran, not that records were cleared", async () => {
  vi.stubGlobal("fetch", () => json({ runs: [] }));
  await render(<WorkspaceRuns />, () => text().includes("還沒有跑過任何 Run"));

  expect(text()).toContain("不是紀錄被清掉了");
});

// --- the own-skills list ----------------------------------------------------

// The two facets every row of this list now carries (04 丙-31). Spread into the
// fixtures below so a test about deletion does not have to restate them, and so
// the fork shape stays written down in exactly one place: **a fork has neither**,
// because it copies the bytes and not the measurement.
const SCANNED = {
  risk: {
    scan_status: "scanned",
    level: "disclosed",
    warnings: 0,
    disclosures: [
      { code: "script-file", label: "含可執行 Script 檔案", note: "平台不曾執行它們。" },
    ],
    note: "來自匯入時的靜態掃描,不執行套件內任何程式碼;開啟 Skill 可看逐項結果。",
  },
  verification: {
    value: "scanned",
    label: "已掃描",
    note: "匯入這個版本時做過靜態掃描,不執行套件內任何程式碼;逐項結果在 Skill 頁面。",
    scanned_at: "2026-08-01T10:00:00Z",
  },
} as const;

const FORKED = {
  risk: {
    scan_status: "unavailable",
    level: "none",
    warnings: 0,
    note: "此結果尚無掃描紀錄,狀態未知——不代表已通過檢查。",
  },
  verification: {
    value: "not_measured",
    label: "未測量",
    note: "這個版本是 Fork 進來的複本,靜態掃描是在來源工作區做的,平台沒有在你的工作區重跑。",
    scanned_at: null,
  },
} as const;

test("WS-004 the own-skills row says whether this skill can be taken away", async () => {
  // 04 丙-31 / 設計 §2.2 in its second direction. `redistribution` and
  // `access_restriction` were on the row ListSkills selects and were dropped in
  // serialisation, so a skill that packaging will refuse looked exactly like one
  // it will not — right up to the packaging screen.
  //
  // ADR-045 changed which value is the common case: a user's own import carries
  // `self_supplied` now and downloads, where it used to carry `unknown` and
  // never could. Both are on this fixture, and they must not render as the same
  // badge — 「可打包下載」 for a self-supplied skill would tell its owner somebody
  // had checked the licence, which nobody did.
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "自己匯入的",
          summary: "一份自己傳上來的套件。",
          redistribution: "self_supplied",
          access_restriction: null,
          ...SCANNED,
        },
        {
          skill_id: "s-3",
          name: "沒人判定過的",
          summary: "目錄裡還沒有人分類的內容。",
          redistribution: "unknown",
          access_restriction: null,
          ...SCANNED,
        },
        {
          skill_id: "s-4",
          name: "平台生成的",
          summary: "依任務描述生成出來的。",
          redistribution: "generated",
          access_restriction: null,
          ...SCANNED,
        },
        {
          skill_id: "s-2",
          name: "Fork 來的",
          summary: "從目錄 Fork 的。",
          redistribution: "allowed",
          access_restriction: null,
          forked_from_skill_id: "s-origin",
          ...FORKED,
        },
      ],
      limit: 100,
      truncated: true,
      total: 137,
    }),
  );
  // Wait on the summary, not the name: 「自己匯入的」 also appears in this page's
  // own intro sentence, so waiting on it resolves before any data arrives.
  await render(<WorkspaceSkills />, () => text().includes("一份自己傳上來的套件"));

  expect(text()).toContain("授權未知，不能打包");
  expect(text()).toContain("可打包下載");
  expect(text()).toContain("可下載（你自己帶進來的）");
  // 0037: the value the server releases and this list refused. The row rendered
  // a red 「授權未知，不能打包」 for a download the platform would have
  // produced — a ternary chain that had never heard of the fifth value.
  expect(text()).toContain("可下載（平台為你生成的）");
  // The contract carried forked_from_* the whole time; a narrower local type for
  // the same endpoint was the app's entire view of it, so the page could not
  // tell a fork from an import even though its own header promised to.
  // 「Fork 自來源 Skill」 since the attribution became a link (ADR-042 決策 6):
  // the id was already on the row and was rendering as an unreachable sentence.
  expect(text()).toContain("Fork 自");
  expect(text()).toContain("自己匯入");
  // §2.2: the 100-row cap was enforced and invisible, so a workspace past it got
  // a short list that read as the whole list.
  expect(text()).toContain("只列出前 100 個");
});

test("WS-004 a forked row says the scan happened somewhere else, not that it passed", async () => {
  // 04 丙-31 / 設計 §2.9. The obvious implementation serves `verified_at` as the
  // newest version's created_at, which is what the search projection means by it
  // — and a fork's newest version row is created the instant somebody presses
  // Fork, with nothing scanned. That would print a timestamp reading as
  // 「剛剛掃過」 on the one case where nothing was measured at all, so the state is
  // named and the timestamp only exists in the state that has one.
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "Fork 來的",
          summary: "從目錄 Fork 的。",
          redistribution: "allowed",
          access_restriction: null,
          forked_from_skill_id: "s-origin",
          ...FORKED,
        },
      ],
      limit: 100,
      truncated: false,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("從目錄 Fork 的"));

  expect(text()).toContain("未測量");
  expect(text()).toContain("靜態掃描是在來源工作區做的");
  // The two facets have to agree: an unscanned row must not also claim the scan
  // found nothing, which is the sentence the scanned-and-clean branch prints.
  expect(text()).not.toContain("未發現警告");
  // §2.9: never a blank, and never a date on a state that has none.
  expect(container.querySelector(".badge-row")?.textContent ?? "").not.toBe("");
});

test("WS-004 a fork of identical bytes shows the source's scan, attributed and dated to the source", async () => {
  // ADR-042 決策 6. The measurement holds for the bytes, not for the location —
  // in-toto binds an attestation to the subject's digest — so a copy whose hash
  // still matches a public-catalogue ancestor carries the ancestor's scan. What
  // is forbidden is doing it silently, so this asserts the three visible halves:
  // the label is not the same as a local scan's, the note names the source, and
  // the date is the source's import rather than the moment of the fork.
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "Fork 來的",
          summary: "從目錄 Fork 的。",
          redistribution: "allowed",
          access_restriction: null,
          forked_from_skill_id: "s-origin",
          risk: SCANNED.risk,
          verification: {
            value: "scanned",
            label: "已掃描（來源）",
            note: "這個版本是 Fork 進來的複本,內容雜湊與來源「PDF Summariser」相同,所以沿用來源匯入時的靜態掃描結果。",
            scanned_at: "2026-07-01T09:00:00Z",
          },
        },
      ],
      limit: 100,
      truncated: false,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("從目錄 Fork 的"));

  expect(text()).toContain("已掃描（來源）");
  expect(text()).toContain("PDF Summariser");
  // The source's import time, older than the fork. An inherited scan showing the
  // fork's own timestamp would read as 「剛剛掃過」, which is the failure the whole
  // named-state design exists to prevent.
  expect(text()).toContain("2026-07-01");
  // The disclosures come across with it, because they are facts about the bytes.
  expect(text()).toContain("含可執行 Script 檔案");
  // Attribution has to be reachable, not just stated.
  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs).toContain("/skills/s-origin");
});

test("WS-004 the own-skills list links each row on to its files and packaging", async () => {
  vi.stubGlobal("fetch", () =>
    json({
      skills: [
        {
          skill_id: SKILL,
          name: "CSV 清理",
          summary: "整理 CSV。",
          redistribution: "allowed",
          access_restriction: null,
          ...SCANNED,
        },
      ],
      limit: 100,
      truncated: false,
    }),
  );
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs).toContain(`/skills/${SKILL}`);
  expect(hrefs).toContain(`/skills/${SKILL}/files`);
  expect(hrefs).toContain(`/skills/${SKILL}/package`);
  // The other workspace lists are reachable from here rather than only from the
  // header, because this is the page a reader lands on looking for "my stuff".
  expect(hrefs).toContain("/workspace/runs");
  expect(hrefs).toContain("/workspace/downloads");
});

test("IA-9 the empty own-skills list offers importing as a link, not as prose", async () => {
  // 資訊架構 §0.1 R3 / §2.3: /workspace/import had exactly one in-page inbound
  // edge (Home's no_results state) and this sentence had been naming importing
  // in prose the whole time — a page that says what to do next and then makes
  // you go find the nav to do it is what R3 counts as one way in.
  //
  // This state is only reachable with a session (GET /skills is RequireSession),
  // which is why the link needs no visitor branch the way Home's exit does.
  vi.stubGlobal("fetch", () => json({ skills: [], limit: 100, truncated: false }));
  await render(<WorkspaceSkills />, () => text().includes("還沒有任何 Skill"));

  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs).toContain("/workspace/import");
  // 設計 §2.1's strong form survives the link: the empty state still says what
  // this empty is NOT.
  expect(text()).toContain("不是清單讀取失敗");
});

// --- deleting a skill (WS-005, 04 丙-22 ①) ----------------------------------

test("WS-005 deleting a skill says what survives it before anything is destroyed", async () => {
  const calls: [string, string | undefined][] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    calls.push([String(input), init?.method]);
    if (init?.method === "DELETE") {
      return json({
        deleted: true,
        versions_retained: 2,
        note: "skill removed from your workspace, lists, and search; version snapshots are retained for the 30-day grace period",
      });
    }
    return json({
      skills: [{ skill_id: SKILL, name: "CSV 清理", summary: "整理 CSV。", ...SCANNED }],
    });
  });
  await render(<WorkspaceSkills />, () => text().includes("CSV 清理"));

  await act(async () => button("刪除")?.click());
  // 02:WS-002 第 3 條: the scope is on screen before the request, and it names
  // what is kept and the forks it does not touch.
  //
  // It used to assert 「先凍結保留一段期間再清除」. That sentence was removed on
  // 2026-08-25 because nothing clears them: the only hard delete of a skill in
  // the repo runs from account deletion, so a skill deleted on its own keeps its
  // rows forever. 02 §2.2 forbids showing a promise nothing enforces, and this
  // page had already had the 30-day numeral struck for the same reason — the
  // clause that outlived it was the one with no number in it.
  expect(text()).toContain("版本快照會凍結保留");
  expect(text()).not.toContain("再清除");
  expect(text()).toContain("別人 Fork 過的版本");
  expect(calls.some(([, method]) => method === "DELETE")).toBe(false);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => calls.some(([, method]) => method === "DELETE"));
  expect(calls.find(([, m]) => m === "DELETE")?.[0]).toContain(`/skills/${SKILL}`);

  // After the fact the server's own note is what is shown — not a second copy of
  // the scope written on this side.
  await waitFor(() => text().includes("version snapshots are retained"));
});

// --- deleting the account (CORE-007, 04 丙-22 ②) -----------------------------

const ME = {
  user_id: "u-1",
  email: "tester@example.com",
  display_name: "tester",
  workspace_id: "ws-1",
  deletion_requested_at: null,
  purge_after: null,
  deletion_scope: null,
};

test("CORE-007 requesting account deletion starts a grace period and shows the server's scope", async () => {
  const calls: [string, string | undefined][] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    calls.push([String(input), init?.method]);
    if (init?.method === "DELETE") {
      return json({
        deletion_requested_at: "2026-08-18T00:00:00Z",
        purge_after: "2026-09-17T00:00:00Z",
        cancellable: true,
        scope: "Your account stays usable until the grace period ends.",
      });
    }
    return json(ME);
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除我的帳號"));

  await act(async () => button("刪除我的帳號")?.click());
  // The pre-scope is about this control, and its whole job is to say that
  // nothing is destroyed yet — the account delete is a countdown, not an act.
  expect(text()).toContain("不會立刻刪掉任何東西");
  expect(calls.some(([, m]) => m === "DELETE")).toBe(false);

  await act(async () => button("確認開始刪除")?.click());
  await waitFor(() => text().includes("Your account stays usable"));
  expect(calls.find(([, m]) => m === "DELETE")?.[0]).toContain("/me");
});

test("CORE-007 a pending deletion is a state with a date and a way out, not a receipt", async () => {
  const posts: string[] = [];
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      posts.push(String(input));
      return json({ deletion_requested_at: null });
    }
    // GET /me after the cancel still answers the pending row; what this test is
    // about is the request going out, not the invalidation round trip.
    return json({
      ...ME,
      deletion_requested_at: "2026-08-18T00:00:00Z",
      purge_after: "2026-09-17T00:00:00Z",
      deletion_scope: "Your account stays usable until the grace period ends.",
    });
  });
  await render(<WorkspaceAccount />, () => text().includes("刪除申請中"));

  // A user who closed the tab after asking has no other place to find either of
  // these, which is what 02:SEC-006「刪除工作具可追蹤狀態」 is asking for.
  expect(text()).toContain("2026-09-17T00:00:00Z");
  expect(text()).toContain("再按一次刪除不會提早");
  // 04 丙-30. Nothing in this test ever calls DELETE /me: this is the reload
  // case, and the scope sentence used to exist only in that one response, so it
  // was gone by the time the user came back to look for it. 設計 §2.8 calls the
  // scope sentence the entire disclosure and §2.10 forbids hiding it, and a
  // disclosure that survives one render is not stated.
  expect(text()).toContain("Your account stays usable until the grace period ends.");

  await act(async () => button("取消刪除申請")?.click());
  await waitFor(() => posts.length > 0);
  expect(posts[0]).toContain("/me/deletion/cancel");
});

// --- the per-download records ----------------------------------------------

const ARTIFACT_ROW = {
  artifact_id: ARTIFACT,
  skill_id: SKILL,
  skill_version_id: "22222222-2222-2222-2222-222222222222",
  target: "standard" as const,
  file_name: "csv-cleanup-v2.zip",
  size_bytes: 4096,
  content_hash: "sha256:bbbb",
  manifest_hash: "sha256:cccc",
  status: "available" as const,
  servable: true,
  serve_state: { value: "available", label: "可下載", note: "" },
  version_number: 2,
  latest_version_number: 5,
  version_state: {
    value: "superseded",
    label: "v2（這個 Skill 已經到 v5）",
    note: "這一份是 v2 的內容,而且不會改變——版本是不可變的。要拿 v5 的內容,回到該 Skill 對 v5 重新打包一次。",
  },
  expires_at: "2099-01-01T00:00:00Z",
  created_at: "2026-08-17T00:00:00Z",
  download_count: 2,
  includes_test_cases: false,
};

test("WS-004 a download row says which version it is and whether a newer one exists", async () => {
  vi.stubGlobal("fetch", () => json({ downloads: [ARTIFACT_ROW] }));
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  // 04 丙-42: the row named its version only as a uuid, and said nothing at all
  // about a newer one existing. 「我下載的是不是最新調整好的那一版」 is the
  // question, and a uuid is not an answer to it however it is arranged.
  expect(text()).toContain("v2（這個 Skill 已經到 v5）");
  // Immutability is the reason, and the reason is what makes the next step
  // obvious: this row will never become v5, so re-package rather than wait.
  expect(text()).toContain("重新打包");
  // Being superseded is not a serving state: these bytes are still on offer,
  // and wanting exactly the version you packaged is legitimate.
  expect(text()).toContain("可下載");
  expect(container.querySelector('a[href*="/content"]')).not.toBeNull();
});

test("WS-004 the download history answers 誰 and 何時 per download, not just a count", async () => {
  const urls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    urls.push(String(input));
    if (String(input).includes("/records")) {
      return json({
        records: [
          { downloaded_at: "2026-08-17T09:00:00Z", actor: "tester" },
          { downloaded_at: "2026-08-17T10:00:00Z", actor: "deleted user" },
        ],
      });
    }
    return json({ downloads: [ARTIFACT_ROW] });
  });
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  // Not fetched until asked for: a history page holds every package this
  // workspace ever built, and one request per row on load would make the
  // cheapest question on the page the most expensive read in the API.
  expect(urls.some((u) => u.includes("/records"))).toBe(false);

  await open("誰下載過");
  await waitFor(() => text().includes("tester"));

  expect(urls.some((u) => u.includes(`/downloads/${ARTIFACT}/records`))).toBe(true);
  expect(text()).toContain("2026-08-17T09:00:00Z");
  // A purged account leaves the row de-identified rather than removing it:
  // "somebody, at this time" is still true.
  expect(text()).toContain("deleted user");
  // CORE-008: this is the product feature, the audit event is the other record.
  expect(text()).toContain("與稽核事件是兩份不同的紀錄");
});

// --- what a run produced, and deleting one of them --------------------------

const RUN_ARTIFACT = {
  artifact_id: "aaaa1111-2222-3333-4444-555566667777",
  file_name: "cleaned.csv",
  content_type: "text/csv",
  size_bytes: 2048,
  content_hash: "sha256:dddd",
  created_at: "2026-08-17T00:03:00Z",
  purged: false,
};

/** Everything the run page reads; the parts this test is not about answer 404. */
function stubRunPage(artifacts: unknown[], onDelete?: (url: string) => void) {
  vi.stubGlobal("fetch", (input: string, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === "DELETE") {
      onDelete?.(url);
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/artifacts")) return json({ artifacts });
    if (url.includes("/trace"))
      return json({
        run_id: RUN,
        status: "succeeded",
        complete: true,
        skills: [],
        resources_read: 0,
        tool_calls: {
          total: 0,
          succeeded: 0,
          failed: 0,
          total_duration_ms: 0,
          slowest_duration_ms: 0,
        },
        errors: [],
        steps: [],
      });
    return json({ error: "not found" }, 404);
  });
}

test("SEC-006 a run output can be deleted, and the scope says what survives it", async () => {
  const deletes: string[] = [];
  stubRunPage([RUN_ARTIFACT], (url) => deletes.push(url));
  await render(<RunTrace />, () => text().includes("cleaned.csv"));

  // The bytes are never offered: the archive is a sandbox's output and the
  // control plane does not open it, so there is no link to invent.
  const hrefs = Array.from(container.querySelectorAll("a")).map(
    (a) => a.getAttribute("href") ?? "",
  );
  expect(hrefs.some((h) => h.includes("/artifacts/"))).toBe(false);
  expect(text()).toContain("控制平面不打開它");

  await act(async () => button("刪除")?.click());
  // 02:WS-002 第 3 條: the scope is on screen before anything is destroyed, and
  // what it promises is that the evaluation keeps its citation.
  expect(text()).toContain("引用過這個檔案的評估不會被改寫");
  expect(deletes).toHaveLength(0);

  await act(async () => button("確認刪除")?.click());
  await waitFor(() => deletes.length > 0);
  expect(deletes[0]).toContain(`/runs/${RUN}/artifacts/${RUN_ARTIFACT.artifact_id}`);
});

test("SEC-006 a purged output keeps its row and says the bytes are gone", async () => {
  stubRunPage([{ ...RUN_ARTIFACT, purged: true, expires_at: "2026-08-16T00:00:00Z" }]);
  await render(<RunTrace />, () => text().includes("cleaned.csv"));

  expect(text()).toContain("檔案已不存在");
  // "It expired" and "it never existed" are different answers; the row is the
  // first one and the empty state is the second.
  expect(text()).toContain("曾經產生過這個檔案」仍然是事實");
});

test("WS-004 a package nobody downloaded says so instead of loading an empty list", async () => {
  const urls: string[] = [];
  vi.stubGlobal("fetch", (input: string) => {
    urls.push(String(input));
    return json({ downloads: [{ ...ARTIFACT_ROW, download_count: 0 }] });
  });
  await render(<Downloads />, () => text().includes("csv-cleanup-v2.zip"));

  await open("誰下載過");

  expect(text()).toContain("還沒有人下載過");
  expect(text()).toContain("建立一個套件不等於取走它");
  expect(urls.some((u) => u.includes("/records"))).toBe(false);
});

// --- WS-005: fork writes to 我的 Skill ---------------------------------------

test("WS-004 a fork invalidates the list it writes to, and does not touch the search key", async () => {
  // The third writer to ["own-skills"] — import and generate are the other two
  // and both invalidate it. Only refetch-on-mount was covering this one, and
  // this app turns focus and reconnect refetch off (api/queryClient).
  vi.stubGlobal("fetch", () => json({ skill_id: "forked-1", version_id: "v1" }, 201));

  // Two cached lists, so the assertion can tell them apart. ["skills"] as the key
  // would reach the second one, and re-running that search makes the server write
  // a second search_performed event (GenerateSkill.tsx records the same trap).
  queryClient.setQueryData(["own-skills"], { skills: [] });
  queryClient.setQueryData(["skills", "search", "pdf"], { results: [] });

  let fork: ReturnType<typeof useForkSkill> | undefined;
  function ForkHarness() {
    fork = useForkSkill();
    return null;
  }
  await render(<ForkHarness />, () => fork !== undefined);
  await act(async () => {
    await fork!.mutateAsync(SKILL);
  });

  expect(queryClient.getQueryState(["own-skills"])?.isInvalidated).toBe(true);
  expect(queryClient.getQueryState(["skills", "search", "pdf"])?.isInvalidated).toBe(false);
});
