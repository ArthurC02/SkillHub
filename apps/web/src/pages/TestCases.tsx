import { Loading } from "../components/Loading";
import { Timestamp, formatAt } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  addCriterion,
  createTestCase,
  deleteCriterion,
  deleteDataset,
  deleteTestCase,
  suggestCriteria,
  updateCriterion,
  updateTestCase,
  useOwnSkills,
  useTestCase,
  useTestCaseDatasets,
  useTestCases,
} from "../api/testcases";
import { useRuns, type RunListItem } from "../api/runs";
import { RunVerdict } from "../components/RunVerdict";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { runStatusLabel } from "./RunEvaluation";
// One byte formatter for the app, not a third copy of the same four lines.
import { bytes } from "./RunPreflight";
import type { AcceptanceCriterion, RubricItem, TestCase } from "../api/testcases";

/**
 * 03:TEST-012 — the Test Case and acceptance-criteria screens.
 *
 * What they close: 02:TEST-001 第 2 條前半 「使用者可純手動建立驗收條件」 and
 * 第 3 條 「使用者可新增、編輯、刪除及確認驗收條件」. The four verbs take a
 * user as their subject, and until this page existed they could only be reached
 * with curl — which is why TEST-003 was handed back to 未勾.
 *
 * The snapshot rule is stated on screen rather than assumed: a run freezes the
 * prompt and the criteria as they stand when it starts, so editing afterwards
 * produces a *new* snapshot for the *next* run and changes nothing about a run
 * that already happened or an evaluation already written (iron rule 4, ADR-003).
 *
 * Confirmation is part of the same statement as the text: changing the wording
 * of a confirmed criterion clears its confirmation, because the agreement was
 * to the old words. The server enforces that; this page says so out loud so the
 * cleared checkbox does not read as a bug.
 */

/**
 * 02:NFR-007「表單具有標籤與清楚的驗證訊息」. The submit button is disabled until
 * the three required fields are filled, and a disabled control with no stated
 * cause reads as a bug — the same ruling the DISC-003 filter bar already follows
 * for its unavailable dimensions. `role="status"` and not `alert`: nothing has
 * gone wrong yet, the form is simply not finished.
 */
function CreateValidation({
  skillId,
  name,
  prompt,
}: {
  skillId: string;
  name: string;
  prompt: string;
}) {
  const missing = [
    skillId === "" ? "選一個 Skill" : "",
    name === "" ? "填名稱" : "",
    prompt.trim() === "" ? "寫 User Prompt" : "",
  ].filter((s) => s !== "");

  if (missing.length === 0) return null;
  return (
    <p className="note" role="status">
      還不能建立，因為：{missing.join("、")}。三個都是必填。
    </p>
  );
}

function criterionState(c: AcceptanceCriterion): string {
  if (c.confirmed_at) return `已確認（${formatAt(c.confirmed_at)}）`;
  return c.source === "suggested" ? "系統建議，尚未確認" : "尚未確認";
}

export function TestCaseList() {
  const navigate = useNavigate();
  const { skill: filter } = useSearch({ from: "/lab/test-cases" });
  const testCases = useTestCases(filter);
  const skills = useOwnSkills();
  const [skillId, setSkillId] = useState("");
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [message, setMessage] = useState("");
  const rows = testCases.data?.pages.flatMap((page) => page.test_cases) ?? [];
  // 丙-116. `?skill=` accepts any UUID (router.tsx only checks the shape), while
  // the create form below only accepts what `GET /skills` returns — so the two
  // halves of this page could disagree about what 「這個 Skill」 means with
  // nothing on screen saying so. The answer costs no request: `useOwnSkills` is
  // already loaded, for the picker. Only decided once that read has landed —
  // before it, `notMine` would be true for every skill including your own.
  const ownedSkill = skills.data?.skills.find((s) => s.skill_id === filter);
  const notMine = Boolean(filter) && Boolean(skills.data) && !ownedSkill;

  const create = useMutation({
    mutationFn: () => createTestCase(skillId, name, prompt),
    onSuccess: (tc) =>
      navigate({ to: "/lab/test-cases/$testCaseId", params: { testCaseId: tc.test_case_id } }),
    onError: (err) => setMessage(err instanceof Error ? err.message : "無法建立 Test Case。"),
  });

  return (
    <section>
      <h1>Test Case</h1>
      <p className="note">
        Test Case 是可編輯的草稿：User Prompt、測試資料與驗收條件。開始 Run 時，平台會把當下的
        Prompt 與驗收條件凍結成快照，之後再改都不會動到已經跑過的 Run。
      </p>

      {/*
        設計 checklist 1 / 義務 §1.2: 這一頁要回答的是「我有哪些 Test Case」,而
        在 375×900 下建立表單把第一列推到 ~y715——答案在 78% 的位置。清單先,
        表單後。§1.3 說這種情況的工具是漸進式揭露而不是模式切換,而把次要動作
        往下移是同一件事最便宜的做法:兩段都還在同一頁,誰都沒有被藏起來。
      */}
      <h2>既有的 Test Case</h2>
      {/*
        The filter is stated whenever it is on, with the way out beside it: a
        list that silently shows a subset reads as "this is everything".
        The skill is named from the rows rather than from a second request —
        every row of a filtered list carries the same `skill_name`.

        丙-116: with no rows there is no `skill_name`, and the fallback used to
        be 「這一個 Skill」 — a filter announcing itself while unable to name what
        it filtered by. `ownedSkill` closes the common half of that (an own skill
        with no test cases yet now gets its real name) and the notice below
        closes the other half, which is the one that mattered: the id belongs to
        a skill outside this workspace, so the list cannot show it AND the picker
        below cannot offer it.
      */}
      {filter && (
        <p className="note" role="status">
          只顯示 <strong>{rows[0]?.skill_name || ownedSkill?.name || "某一個 Skill"}</strong>{" "}
          的 Test Case。{" "}
          <Link to="/lab/test-cases" search={{ skill: undefined }}>
            顯示全部
          </Link>
        </p>
      )}
      {notMine && (
        <p className="notice" role="status">
          這個 Skill 不在你的工作區。Test Case 屬於工作區，所以這裡沒有東西可以顯示，
          下面建立表單的 Skill 選單也選不到它——
          <Link to="/skills/$skillId" params={{ skillId: filter as string }}>
            先把它 Fork 一份
          </Link>
          ，才會有屬於你的版本可以建立 Test Case。
        </p>
      )}
      {testCases.isPending && <Loading what=" Test Case 清單" />}
      <ReadFailure error={testCases.error} what=" Test Case" />
      {testCases.data &&
        (rows.length === 0 ? (
          // Not when the skill is somebody else's: the notice above already said
          // why this is empty, and 「還沒有」 there would read as an invitation to
          // create one — the exact sentence that ended the corridor.
          notMine ? null : (
            <p>{filter ? "這個 Skill 還沒有 Test Case。" : "還沒有 Test Case。"}</p>
          )
        ) : (
          <ul className="search-results">
            {rows.map((tc) => (
              <li key={tc.test_case_id} className="search-result">
                <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: tc.test_case_id }}>
                  {tc.name}
                </Link>
                <p className="note">
                  {/* Empty means the skill is no longer visible to the caller —
                      "we cannot name it", which is not a name and not a UUID. */}
                  Skill：
                  {tc.skill_name === ""
                    ? "無權檢視（這個 Skill 已不在你的可見範圍）"
                    : tc.skill_name}
                </p>
                <p className="note">
                  驗收條件已確認 {tc.criteria_confirmed}/{tc.criteria_total} 條 · Rubric{" "}
                  {tc.has_rubric ? "有" : "無"} · 最後修改 <Timestamp at={tc.updated_at} />
                </p>
              </li>
            ))}
          </ul>
        ))}
      {testCases.hasNextPage && (
        <button
          type="button"
          disabled={testCases.isFetchingNextPage}
          onClick={() => testCases.fetchNextPage()}
        >
          {testCases.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}

      <h2>建立新的 Test Case</h2>
      <ReadFailure error={skills.error} what="你的 Skill 清單" />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <p className="field">
          <label htmlFor="tc-skill">Skill</label>
          <select id="tc-skill" value={skillId} onChange={(e) => setSkillId(e.target.value)}>
            <option value="">請選擇</option>
            {skills.data?.skills.map((s) => (
              <option key={s.skill_id} value={s.skill_id}>
                {s.name}
              </option>
            ))}
          </select>
        </p>
        <p className="field">
          <label htmlFor="tc-name">名稱</label>
          <input id="tc-name" value={name} onChange={(e) => setName(e.target.value)} size={40} />
        </p>
        <p className="field">
          <label htmlFor="tc-prompt">User Prompt</label>
          <textarea
            id="tc-prompt"
            rows={4}
            cols={60}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
          />
        </p>
        <CreateValidation skillId={skillId} name={name} prompt={prompt} />
        <button
          type="submit"
          disabled={create.isPending || skillId === "" || name === "" || prompt.trim() === ""}
        >
          建立
        </button>
      </form>
      {message && <p role="alert">{message}</p>}
    </section>
  );
}

export function TestCaseDetail() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId" });
  const testCase = useTestCase(testCaseId);
  // One read serves two things: the 執行歷史 section below, and the version the
  // 開始試跑 link opens the picker on. The "last version run" is a fact about
  // the run history, so it is read from there rather than copied into the draft.
  const runs = useRuns(testCaseId);
  const [deleted, setDeleted] = useState<{ datasets_deleted: number } | null>(null);

  if (deleted) {
    return (
      <section>
        <h1>已刪除這個 Test Case</h1>
        {/* WS-002 「系統應說明刪除範圍」: the count is the server's, not a guess,
            and what survived is named as plainly as what went. */}
        <p role="status">
          草稿與它的 {deleted.datasets_deleted} 個上傳檔案都已刪除，檔案本體也已移除。
        </p>
        <p className="note">
          <strong>快照與歷史 Run 不受影響</strong>
          ：已經跑過的 Run 仍保留當時凍結的 Prompt、驗收條件，以及每個檔案的名稱與內容雜湊，所以那些
          Run 仍可追溯，只是不再可重現。
        </p>
        <p>
          <Link to="/lab/test-cases">回到 Test Case 列表</Link>
        </p>
      </section>
    );
  }

  if (testCase.isPending) return <Loading what=" Test Case " />;
  if (testCase.error) {
    // 404 stays this page's own answer — the id is wrong, and no login fixes
    // that. Everything else goes through the shared 401 handling (資訊架構 IA-6).
    if (testCase.error instanceof ApiError && testCase.error.status === 404) {
      return <p role="alert">找不到這個 Test Case。</p>;
    }
    return <ReadFailure error={testCase.error} what=" Test Case" />;
  }

  const history = runs.data?.pages.flatMap((page) => page.runs) ?? [];
  const lastVersion = history[0]?.skill_version_id;

  return (
    <section key={testCaseId}>
      <h1>{testCase.data.name}</h1>
      <p className="note">
        <Link to="/lab/test-cases" search={{ skill: testCase.data.skill_id }}>
          回到這個 Skill 的 Test Case 列表
        </Link>
      </p>
      <PromptForm testCase={testCase.data} />
      <CriteriaSection testCase={testCase.data} />
      <RubricSection testCase={testCase.data} />
      <DatasetSection testCaseId={testCaseId} />
      <h2>開始試跑</h2>
      <p className="note">
        <Link
          to="/lab/run"
          search={{ skill: testCase.data.skill_id, test_case: testCaseId, version: lastVersion }}
        >
          前往執行前權限確認
        </Link>
        （要跑哪一個 Skill Version 在那個頁面上選
        {lastVersion ? "，預設是這個 Test Case 上次跑的那一版" : ""}）。開始 Run
        前一定會再顯示一次權限摘要並要求確認。
      </p>
      <RunHistory runs={runs} history={history} />
      <DeleteTestCase testCaseId={testCaseId} onDeleted={setDeleted} />
    </section>
  );
}

/**
 * The return leg of 建立 → 試跑 → 回來看, served by GET /runs?test_case_id=.
 *
 * `status` is worded as execution and never as a pass, the same ruling the run
 * history page keeps (ADR-025): what finished is the workload, and whether the
 * task was done is the evaluation's verdict on the run's own page.
 */
function RunHistory({
  runs,
  history,
}: {
  runs: ReturnType<typeof useRuns>;
  history: RunListItem[];
}) {
  return (
    <>
      <h2>執行歷史</h2>
      {/*
        設計 §2.5 / ADR-025 說兩軸,判定排在前面。這份清單一直只有一軸,因為
        `RunListItem` 沒有判定欄位——那是契約缺口不是排版問題,已於 04 丙-32
        補上。這句話留著:順序本身教不會任何人兩軸的差別。
      */}
      <p className="note">
        每一列兩軸：<strong>任務判定在前，執行狀態在後</strong>
        ——後者只說工作負載跑完了沒有。逐條驗收結果在各自的 Run 頁面上。
      </p>
      {runs.isPending && <Loading what="執行歷史" />}
      <ReadFailure error={runs.error} what="執行歷史" />
      {runs.data &&
        (history.length === 0 ? (
          <p>尚無執行。這個 Test Case 還沒有跑過任何 Run。</p>
        ) : (
          <ul className="download-list">
            {history.map((run) => (
              <li key={run.run_id} className="download-item">
                <p>
                  <Link to="/runs/$runId" params={{ runId: run.run_id }}>
                    <Timestamp at={run.created_at} />
                  </Link>
                </p>
                <p className="badge-row">
                  <RunVerdict verdict={run.evaluation} />
                </p>
                <p className="badge-row">
                  <span className="badge">執行狀態：{runStatusLabel(run.status)}</span>
                </p>
                {/* 義務 §1.1: 這兩個欄位一直在 RunListItem 裡而從來沒有被畫
                    出來,所以一列「執行失敗」講不出為什麼失敗。伺服器沒給就
                    不編一個理由出來——不渲染,而不是渲染成空白。 */}
                {(run.status_reason || run.failure_class) && (
                  <p className="note">
                    {run.status_reason ?? "未測量（伺服器沒有回報原因）"}
                    {run.failure_class && `（分類：${run.failure_class}）`}
                  </p>
                )}
                <p className="note">
                  Skill Version <code>{run.skill_version_id}</code>
                  {run.finished_at ? (
                    <>
                      ｜結束於 <Timestamp at={run.finished_at} />
                    </>
                  ) : (
                    "｜尚未結束"
                  )}
                </p>
              </li>
            ))}
          </ul>
        ))}
      {runs.hasNextPage && (
        <button
          type="button"
          disabled={runs.isFetchingNextPage}
          onClick={() => runs.fetchNextPage()}
        >
          {runs.isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </>
  );
}

/**
 * 02:WS-002 第 3 條 — the delete, with its scope stated before it runs and the
 * server's own count stated after. Uses the shared two-step control so this is
 * not a fourth copy of the same markup (04 丙-22).
 */
function DeleteTestCase({
  testCaseId,
  onDeleted,
}: {
  testCaseId: string;
  onDeleted: (result: { datasets_deleted: number }) => void;
}) {
  const client = useQueryClient();
  const [message, setMessage] = useState("");

  const remove = useMutation({
    mutationFn: () => deleteTestCase(testCaseId),
    onSuccess: async (result) => {
      // The whole ["test-cases"] subtree: this draft's own read, every filtered
      // list, and the unfiltered one. Narrower keys would leave the list the
      // user is about to land on still showing the row that just went.
      await client.invalidateQueries({ queryKey: ["test-cases"] });
      onDeleted(result);
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "刪除失敗。"),
  });

  return (
    <>
      <h2>刪除這個 Test Case</h2>
      <p>
        <ConfirmDelete
          scopeId={`delete-scope-${testCaseId}`}
          scope="會刪掉這個草稿與它已上傳的檔案。已經跑過的 Run 及其快照不受影響——那是那些 Run 執行內容的紀錄。"
          pending={remove.isPending}
          onAsk={() => setMessage("")}
          onConfirm={() => remove.mutate()}
          // Distinct from the 刪除 on every criterion and every uploaded file:
          // three unlabelled 刪除 buttons on one page is three ways to destroy
          // three different things, told apart only by position.
          label="刪除整個 Test Case"
          confirmLabel="確認刪除整個 Test Case"
        />
      </p>
      {message && <p role="alert">{message}</p>}
    </>
  );
}

function PromptForm({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const [name, setName] = useState(testCase.name);
  const [prompt, setPrompt] = useState(testCase.user_prompt);
  const [message, setMessage] = useState("");

  const save = useMutation({
    mutationFn: () => updateTestCase(testCase.test_case_id, { name, user_prompt: prompt }),
    onSuccess: async () => {
      setMessage("已儲存。");
      await client.invalidateQueries({ queryKey: ["test-cases"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "儲存失敗。"),
  });

  return (
    <>
      <h2>名稱與 User Prompt</h2>
      <p className="field">
        <label htmlFor="edit-name">名稱</label>
        <input id="edit-name" value={name} onChange={(e) => setName(e.target.value)} size={40} />
      </p>
      <p className="field">
        <label htmlFor="edit-prompt">User Prompt</label>
        <textarea
          id="edit-prompt"
          rows={5}
          cols={60}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
        />
      </p>
      <button
        type="button"
        disabled={save.isPending || prompt.trim() === "" || name.trim() === ""}
        onClick={() => save.mutate()}
      >
        儲存
      </button>{" "}
      {/* 設計 §2.4 — 停用要說原因,原因是看得見的文字而不是 title。同一頁上方的
          CreateValidation 已經是這個形狀,這裡往下套用同一個。 */}
      {(name.trim() === "" || prompt.trim() === "") && (
        <span className="note" role="status">
          還不能儲存，因為：
          {[
            name.trim() === "" ? "名稱是空的" : "",
            prompt.trim() === "" ? "User Prompt 是空的" : "",
          ]
            .filter((s) => s !== "")
            .join("、")}
          。兩個都是必填。
        </span>
      )}
      {message && <p role="status">{message}</p>}
    </>
  );
}

function CriteriaSection({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const [text, setText] = useState("");
  const [message, setMessage] = useState("");
  // Proposals, held on the client only. Nothing was written by asking, so
  // walking away from this list leaves the draft exactly as it was.
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const refresh = () => client.invalidateQueries({ queryKey: ["test-cases"] });

  const add = useMutation({
    mutationFn: () => addCriterion(testCase.test_case_id, text),
    onSuccess: async () => {
      setText("");
      setMessage("");
      await refresh();
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "無法新增驗收條件。"),
  });

  const suggest = useMutation({
    mutationFn: () => suggestCriteria(testCase.test_case_id),
    onSuccess: (res) => {
      setSuggestions(res.suggestions.map((s) => s.text));
      setMessage(res.suggestions.length === 0 ? "這次沒有可用的建議，請自己手動輸入。" : "");
    },
    onError: (err) =>
      setMessage(
        err instanceof ApiError && err.status === 503
          ? `目前無法自動建議（${err.message}）。驗收條件可以自己手動輸入，不需要這個功能。`
          : err instanceof Error
            ? err.message
            : "無法取得建議。",
      ),
  });

  // Adoption is the ordinary add route with the wording labelled as a model's,
  // one at a time: the user decides which proposals become criteria, and each
  // one still arrives unconfirmed because adopting a wording is not agreeing
  // to it (TEST-001 確認權在使用者).
  const adopt = useMutation({
    mutationFn: (proposal: string) =>
      addCriterion(testCase.test_case_id, proposal, "suggested").then(() => proposal),
    onSuccess: async (proposal) => {
      setSuggestions((prev) => prev.filter((s) => s !== proposal));
      setMessage("");
      await refresh();
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "無法採納這條建議。"),
  });

  return (
    <>
      <h2>驗收條件</h2>
      <p className="note">
        每一條都會被逐項判定為通過／未通過／無法判斷。 開始 Run
        時，這裡的內容會被凍結成快照：之後修改只影響<strong>下一次</strong>
        Run，不會改寫任何已經完成的 Run 或已經寫好的評估。
      </p>

      {testCase.acceptance_criteria.length === 0 ? (
        <p>還沒有驗收條件。沒有驗收條件的 Run 沒有可逐條判定的依據。</p>
      ) : (
        <ul className="criterion-list">
          {testCase.acceptance_criteria.map((c) => (
            <CriterionRow key={c.id} testCaseId={testCase.test_case_id} criterion={c} />
          ))}
        </ul>
      )}

      <p>
        <label htmlFor="new-criterion">新增驗收條件</label>{" "}
        <input
          id="new-criterion"
          value={text}
          onChange={(e) => setText(e.target.value)}
          size={50}
          maxLength={2000}
        />{" "}
        <button
          type="button"
          disabled={add.isPending || text.trim() === ""}
          onClick={() => add.mutate()}
        >
          新增
        </button>{" "}
        <button type="button" disabled={suggest.isPending} onClick={() => suggest.mutate()}>
          請系統建議（選用）
        </button>{" "}
        {/* 設計 §2.4. */}
        {text.trim() === "" && (
          <span className="note" role="status">
            還不能新增，因為左邊的欄位是空的。
          </span>
        )}
      </p>
      {message && <p role="alert">{message}</p>}

      {suggestions.length > 0 && (
        <>
          <h3>系統的建議（尚未加入）</h3>
          <p className="note">
            以下只是建議，還沒有寫進這個 Test Case。按「採納」才會加成一條驗收條件，並且會標成
            系統建議、維持未確認——要不要算數還是你決定。不想要就按「忽略」，什麼都不會發生。
          </p>
          <ul className="criterion-list">
            {suggestions.map((s) => (
              <li key={s} className="criterion">
                <p>{s}</p>
                <p>
                  <button type="button" disabled={adopt.isPending} onClick={() => adopt.mutate(s)}>
                    採納
                  </button>{" "}
                  <button
                    type="button"
                    onClick={() => setSuggestions((prev) => prev.filter((x) => x !== s))}
                  >
                    忽略
                  </button>
                </p>
              </li>
            ))}
          </ul>
        </>
      )}
    </>
  );
}

function CriterionRow({
  testCaseId,
  criterion,
}: {
  testCaseId: string;
  criterion: AcceptanceCriterion;
}) {
  const client = useQueryClient();
  const [draft, setDraft] = useState(criterion.text);
  const [message, setMessage] = useState("");
  const refresh = () => client.invalidateQueries({ queryKey: ["test-cases"] });

  const mutate = useMutation({
    mutationFn: (action: "save" | "confirm" | "unconfirm" | "delete") => {
      if (action === "delete") return deleteCriterion(testCaseId, criterion.id);
      if (action === "save") return updateCriterion(testCaseId, criterion.id, { text: draft });
      return updateCriterion(testCaseId, criterion.id, { confirmed: action === "confirm" });
    },
    onSuccess: async () => {
      setMessage("");
      await refresh();
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "操作失敗。"),
  });

  const edited = draft !== criterion.text;

  return (
    <li className="criterion">
      <label htmlFor={`criterion-${criterion.id}`} className="note">
        驗收條件
      </label>{" "}
      <input
        id={`criterion-${criterion.id}`}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        size={50}
        maxLength={2000}
      />
      <p className="note">狀態：{criterionState(criterion)}</p>
      {/*
        設計 §2.4 — 停用要說原因，而且原因不能只活在 title 裡。這一句原本掛在
        `edited && criterion.confirmed_at`，也就是只在「已確認」那一支渲染；而被
        停用的是另一支的「確認」鈕。使用者在一條未確認的條件上打字、按鈕變灰、
        畫面上沒有一個字說為什麼——正是 §2.4 命名的那個失效。兩支各有自己的話，
        停用的那一支再以 aria-describedby 綁到按鈕上（§2.10 第 5 項：理由不折疊）。
      */}
      {edited && (
        <p className="note" id={`criterion-edited-${criterion.id}`}>
          {criterion.confirmed_at
            ? "改動文字後儲存會清除這一條的確認，因為當初確認的是舊的文字。"
            : "文字改了還沒儲存，所以現在不能確認——確認的必須是已經存下來的那一句。先按「儲存文字」。"}
        </p>
      )}
      <p>
        <button
          type="button"
          disabled={mutate.isPending || !edited || draft.trim() === ""}
          onClick={() => mutate.mutate("save")}
        >
          儲存文字
        </button>{" "}
        {criterion.confirmed_at ? (
          <button
            type="button"
            disabled={mutate.isPending}
            onClick={() => mutate.mutate("unconfirm")}
          >
            取消確認
          </button>
        ) : (
          <button
            type="button"
            disabled={mutate.isPending || edited}
            aria-describedby={edited ? `criterion-edited-${criterion.id}` : undefined}
            onClick={() => mutate.mutate("confirm")}
          >
            確認
          </button>
        )}{" "}
        {/*
          設計 §2.8 — 毀滅性動作兩段式,而範圍那句話就是整個揭露。驗收條件是使用者
          自己寫的文字,原本是一顆一鍵即毀、沒有範圍句也沒有 aria-describedby 的
          按鈕,與同一個檔案裡「刪除整個 Test Case」用的是兩套機制。四段照
          WorkspaceSkills 的範本:消失什麼、還救得回來多久、什麼不受影響、還要另外
          刪什麼。
        */}
        <ConfirmDelete
          scopeId={`criterion-delete-scope-${criterion.id}`}
          scope={
            <>
              會刪掉這一條驗收條件的文字與它的確認狀態，之後的 Run
              不再逐條判定它。這一條沒有暫存區也沒有保留期，按下去就沒有了，救不回來。 已經跑過的
              Run 與已經寫好的評估不受影響——它們判定的是當時凍結的快照，那一份仍然 留著這一條。
              掛在這一條上的 rubric 說明要另外處理，在下面的「Rubric（選用）」那一節。
            </>
          }
          pending={mutate.isPending}
          onAsk={() => setMessage("")}
          onConfirm={() => mutate.mutate("delete")}
          // Distinct from 刪除整個 Test Case and from a file's 刪除: three
          // unlabelled 刪除 on one page is three different destructions told
          // apart only by position.
          label="刪除這一條"
          confirmLabel="確認刪除這一條"
        />
      </p>
      {/*
        設計 §2.4 — this pair is the worst of the four on this screen, because
        neither condition can be read off the screen: 儲存文字 is dead when the
        box matches what is stored, and 確認 is dead when it does not.
      */}
      {!edited && (
        <p className="note" role="status">
          「儲存文字」現在不能按，因為文字和已儲存的內容一樣，沒有變更要存。
        </p>
      )}
      {edited && draft.trim() === "" && (
        <p className="note" role="status">
          「儲存文字」現在不能按，因為驗收條件不能是空白。
        </p>
      )}
      {edited && !criterion.confirmed_at && (
        <p className="note" role="status">
          「確認」現在不能按，因為文字改過還沒儲存——確認的是已儲存的那段文字，請先按「儲存文字」。
        </p>
      )}
      {message && <p role="alert">{message}</p>}
    </li>
  );
}

/**
 * CONTENT-007's rubric editor.
 *
 * One row per acceptance criterion, and not a free-standing list of items,
 * because a rubric item is addressed by the criterion it strengthens: the judge
 * answers one verdict per criterion id and the platform drops any id it did not
 * send, so an item that names anything else is an item whose answer has nowhere
 * to be stored. Laying the editor out this way makes that impossible to get
 * wrong instead of explaining it afterwards in an error message.
 */
function RubricSection({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const stored = testCase.rubric;
  const [version, setVersion] = useState(stored?.version ?? "");
  const [items, setItems] = useState<Record<string, RubricItem>>(
    Object.fromEntries((stored?.items ?? []).map((i) => [i.id, i])),
  );
  const [message, setMessage] = useState("");

  const save = useMutation({
    mutationFn: () => {
      const list = testCase.acceptance_criteria
        .map((c) => items[c.id])
        .filter((i): i is RubricItem => i !== undefined && i.text.trim() !== "");
      // No items is not an empty rubric, it is no rubric. Sending null says so;
      // sending `{items: []}` would be a rubric that says nothing.
      return updateTestCase(testCase.test_case_id, {
        rubric: list.length === 0 ? null : { version: version.trim(), items: list },
      });
    },
    onSuccess: async () => {
      setMessage("已儲存。");
      await client.invalidateQueries({ queryKey: ["test-cases"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "儲存失敗。"),
  });

  const update = (id: string, patch: Partial<RubricItem>) =>
    setItems((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? { id, text: "", evidence_required: false }), ...patch },
    }));

  const used = testCase.acceptance_criteria.filter((c) => items[c.id]?.text.trim()).length;

  return (
    <>
      {/*
        設計 checklist 6 — `h3`, not a second `h2`. Rubric is a child of 驗收條件
        by this section's own copy (「每一條都掛在上面某一條驗收條件上」) and by
        its own code: it refuses to render an editor at all when the criteria
        list is empty. axe never sees this — `heading-order` fails a skipped
        level, not a level that should have gone down and didn't (§6).
      */}
      <h3>Rubric（選用）</h3>
      <p className="note">
        Rubric 是驗收條件的<strong>加強說法</strong>，不是另一套判定：每一條都掛在上面某一條驗收
        條件上，只是額外說明「做到什麼程度算過」以及「要不要引原文」。權重只是給模型看的相對
        重要性，平台不拿它算分。開始 Run 時 rubric 會跟驗收條件一起凍結成快照，之後修改只影響
        <strong>下一次</strong> Run。
      </p>
      {testCase.acceptance_criteria.length === 0 ? (
        <p>要先有驗收條件才能寫 rubric——rubric 的每一條都是掛在某一條驗收條件上的。</p>
      ) : (
        <>
          <p>
            <label htmlFor="rubric-version">Rubric 版本</label>{" "}
            <input
              id="rubric-version"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              size={40}
              maxLength={200}
              placeholder="例如 content-007/writing/v1"
            />
            <br />
            <span className="note">
              改任何一條的文字、權重或引文要求就是新版本；評估報告會記下這次判定是在哪個版本下做的。
            </span>
          </p>
          <ul className="criterion-list">
            {testCase.acceptance_criteria.map((c) => {
              const item = items[c.id];
              return (
                <li key={c.id} className="criterion">
                  <p className="note">驗收條件：{c.text}</p>
                  <label htmlFor={`rubric-${c.id}`} className="note">
                    這一條的 rubric 說明（留空＝這條沒有 rubric）
                  </label>
                  <br />
                  <textarea
                    id={`rubric-${c.id}`}
                    rows={3}
                    cols={60}
                    maxLength={2000}
                    value={item?.text ?? ""}
                    onChange={(e) => update(c.id, { text: e.target.value })}
                  />
                  <p>
                    <label htmlFor={`rubric-weight-${c.id}`}>權重</label>{" "}
                    <input
                      id={`rubric-weight-${c.id}`}
                      type="number"
                      min={0}
                      step={1}
                      size={4}
                      value={item?.weight ?? ""}
                      onChange={(e) =>
                        update(c.id, {
                          weight: e.target.value === "" ? undefined : Number(e.target.value),
                        })
                      }
                    />{" "}
                    <label htmlFor={`rubric-evidence-${c.id}`}>
                      <input
                        id={`rubric-evidence-${c.id}`}
                        type="checkbox"
                        checked={item?.evidence_required ?? false}
                        onChange={(e) => update(c.id, { evidence_required: e.target.checked })}
                      />{" "}
                      要求引出原文
                    </label>
                  </p>
                </li>
              );
            })}
          </ul>
          <button
            type="button"
            disabled={save.isPending || (used > 0 && version.trim() === "")}
            onClick={() => save.mutate()}
          >
            儲存 Rubric
          </button>{" "}
          <span className="note">
            {used === 0
              ? "目前沒有任何一條有內容，儲存等於移除這個 Test Case 的 rubric。"
              : `目前 ${used} 條有內容。`}
          </span>
          {/* 設計 §2.4 — 這是這一頁第五個停用的控制項，也是唯一一個沒有說原因的：
              上面那句只報「幾條有內容」，讀者看到按不下去的按鈕會以為是壞掉。
              同一頁另外四處（CreateValidation、新增驗收條件、儲存文字／確認）已經是
              這個形狀。 */}
          {used > 0 && version.trim() === "" && (
            <span className="note" role="status">
              還不能儲存，因為 Rubric 版本是空的。有內容的 rubric
              一定要有版本——評估報告要記下這次判定是在哪個版本下做的。
            </span>
          )}
          {message && <p role="status">{message}</p>}
        </>
      )}
    </>
  );
}

function DatasetSection({ testCaseId }: { testCaseId: string }) {
  const client = useQueryClient();
  const datasets = useTestCaseDatasets(testCaseId);
  const [message, setMessage] = useState("");

  const remove = useMutation({
    mutationFn: (datasetId: string) => deleteDataset(testCaseId, datasetId),
    onSuccess: async (result) => {
      setMessage(result.note);
      await client.invalidateQueries({ queryKey: ["test-cases", testCaseId, "datasets"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "刪除失敗。"),
  });

  return (
    <>
      <h2>測試資料</h2>
      <p className="note">
        <Link to="/lab/datasets" search={{ test_case: testCaseId }}>
          上傳檔案
        </Link>
        （上傳規則會在選檔前顯示）。
      </p>
      {datasets.isPending && <Loading what="檔案清單" />}
      <ReadFailure error={datasets.error} what="檔案清單" />
      {datasets.data &&
        (datasets.data.datasets.length === 0 ? (
          <p>還沒有上傳任何檔案。</p>
        ) : (
          <>
            {/* 義務 §1.1: 上傳頁報的是上限,而這裡是唯一知道「已經用掉多少」的
                地方。size_bytes 一直在型別裡而從來沒有被畫出來,所以讀者無從
                拿一個檔案去對那兩個上限。 */}
            <p className="note">
              目前 {datasets.data.datasets.length} 個檔案，合計 {bytes(datasets.data.total_bytes)}
              。上限在上傳頁的「大小限制」。
            </p>
            <ul className="file-tree">
              {datasets.data.datasets.map((d) => (
                <li key={d.dataset_id}>
                  {d.file_name}{" "}
                  <span className="note">
                    （{d.content_type}・{bytes(d.size_bytes)}）
                  </span>{" "}
                  {/*
                    設計 §2.8 — 同一條規則,不同的範圍句。刪掉的是檔案本體,而
                    「已跑過的 Run 仍保留它的名稱與內容雜湊」是伺服器自己講的
                    (trial/design/dataset.go DeleteDataset 與該端點回的 note)。
                  */}
                  <ConfirmDelete
                    scopeId={`dataset-delete-scope-${d.dataset_id}`}
                    scope={
                      <>
                        會刪掉 <strong>{d.file_name}</strong> 的檔案本體，儲存的位元組會被移除，
                        之後的 Run 讀不到它。沒有回收桶也沒有保留期，刪了就沒有備份可以還原。
                        已經跑過的 Run 不受影響——它們的快照仍保留這個檔案的名稱與內容雜湊， 所以那些
                        Run 仍可追溯，只是不再可重現。這個 Test Case
                        本身與其他檔案都還在；要連草稿一起刪，在這一頁最下面的「刪除這個 Test
                        Case」。
                      </>
                    }
                    pending={remove.isPending}
                    onAsk={() => setMessage("")}
                    onConfirm={() => remove.mutate(d.dataset_id)}
                    label="刪除這個檔案"
                    confirmLabel="確認刪除這個檔案"
                  />
                  {/* TEST-002 的保存政策，落到這一個檔案上。不編一個到期日出來；
                      設計 §2.9 的表列詞是「未測量」（伺服器沒回報）。 */}
                  <p className="note">
                    {d.expires_at ? (
                      <>
                        保存到 <Timestamp at={d.expires_at} /> 自動刪除
                      </>
                    ) : (
                      "到期日未測量"
                    )}
                  </p>
                </li>
              ))}
            </ul>
          </>
        ))}
      {message && <p role="status">{message}</p>}
    </>
  );
}
