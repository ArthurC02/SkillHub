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
import { ListFreshness } from "../components/ListFreshness";
import { IN_FLIGHT_RUN_STATUSES } from "../api/trace";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { runStatusLabel } from "./RunEvaluation";
import { bytes } from "./RunPreflight";
import type { AcceptanceCriterion, RubricItem, TestCase } from "../api/testcases";

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
    <p className="note" role="status" id="create-why">
      還不能建立，因為：{missing.join("、")}。三個都是必填。
    </p>
  );
}

const MAX_NAME_BYTES = 200;
const MAX_PROMPT_BYTES = 32768;
const MAX_CRITERIA = 50;

// UTF-8 byte length, not JS string length (UTF-16 units) — the server's limit is bytes.
function byteLength(s: string): number {
  return new TextEncoder().encode(s).length;
}

function oversizeReason(name: string, prompt: string): string | null {
  const n = byteLength(name);
  if (n > MAX_NAME_BYTES) return `名稱最多 ${MAX_NAME_BYTES} bytes，目前 ${n} bytes。`;
  const p = byteLength(prompt);
  if (p > MAX_PROMPT_BYTES) return `Prompt 最多 ${MAX_PROMPT_BYTES} bytes，目前 ${p} bytes。`;
  return null;
}

function MutationError({
  error,
  what,
  fallback,
  serverSaysStatuses = [],
}: {
  error: unknown;
  what: string;
  fallback: string;
  serverSaysStatuses?: number[];
}) {
  return (
    <ReadFailure error={error} what={what}>
      <p role="alert">
        {error instanceof ApiError && serverSaysStatuses.includes(error.status)
          ? error.message
          : fallback}
      </p>
    </ReadFailure>
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
  const [createError, setCreateError] = useState<unknown>(null);
  const sizeReason = oversizeReason(name, prompt);
  const rows = testCases.data?.pages.flatMap((page) => page.test_cases) ?? [];
  const ownedSkill = skills.data?.skills.find((s) => s.skill_id === filter);
  const notMine = Boolean(filter) && Boolean(skills.data) && !ownedSkill;
  const chosenSkill = skillId || ownedSkill?.skill_id || "";

  const create = useMutation({
    mutationFn: () => createTestCase(chosenSkill, name, prompt),
    onSuccess: (tc) => {
      setCreateError(null);
      navigate({ to: "/lab/test-cases/$testCaseId", params: { testCaseId: tc.test_case_id } });
    },
    onError: (err) => setCreateError(err),
  });

  return (
    <section>
      <h1>Test Case</h1>
      <p className="note" data-role="teaching">
        Test Case 是可編輯的草稿：User Prompt、測試資料與驗收條件。
      </p>

      <h2>既有的 Test Case</h2>
      {filter && (
        <p className="note" role="status">
          只顯示 <strong>{rows[0]?.skill_name || ownedSkill?.name || "某一個 Skill"}</strong> 的
          Test Case。{" "}
          <Link to="/lab/test-cases" search={{ skill: undefined }}>
            顯示全部
          </Link>
        </p>
      )}
      {notMine && (
        <p className="notice" role="status">
          這個 Skill 不在你的工作區。Test Case 屬於工作區，所以這裡看不到它，建立表單的 Skill
          選單也選不到它——
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
                  Skill：
                  {tc.skill_name === ""
                    ? "這個 Skill 已經不在你的清單裡（已刪除，或已下架）"
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
          if (oversizeReason(name, prompt)) return;
          create.mutate();
        }}
      >
        <p className="field">
          <label htmlFor="tc-skill">Skill</label>
          <select id="tc-skill" value={chosenSkill} onChange={(e) => setSkillId(e.target.value)}>
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
          <input
            id="tc-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            size={40}
          />{" "}
          <span className="note">名稱最多 {MAX_NAME_BYTES} bytes。</span>
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
          <br />
          <span className="note">User Prompt 最多 {MAX_PROMPT_BYTES} bytes。</span>
        </p>
        <CreateValidation skillId={chosenSkill} name={name} prompt={prompt} />
        {sizeReason && (
          <p className="note" role="status" id="create-size-why">
            {sizeReason}
          </p>
        )}
        <button
          type="submit"
          className="action"
          disabled={
            create.isPending ||
            chosenSkill === "" ||
            name.trim() === "" ||
            prompt.trim() === "" ||
            sizeReason !== null
          }
          aria-describedby={
            chosenSkill === "" || name.trim() === "" || prompt.trim() === ""
              ? "create-why"
              : sizeReason
                ? "create-size-why"
                : undefined
          }
        >
          {create.isPending ? "建立中…" : "建立"}
        </button>
      </form>
      <MutationError
        error={createError}
        what="Test Case"
        fallback="建立沒有成功，可以再按一次。"
        serverSaysStatuses={[400]}
      />
    </section>
  );
}

export function TestCaseDetail() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId" });
  const testCase = useTestCase(testCaseId);
  const runs = useRuns(testCaseId);
  const [deleted, setDeleted] = useState<{ datasets_deleted: number } | null>(null);

  if (deleted) {
    return (
      <section>
        <h1>已刪除這個 Test Case</h1>
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
      <p>
        <Link
          className="action"
          to="/lab/run"
          search={{ skill: testCase.data.skill_id, test_case: testCaseId, version: lastVersion }}
        >
          前往執行前權限確認
        </Link>
      </p>
      <p className="note" data-role="teaching">
        （要跑哪一個 Skill Version 在那個頁面上選
        {lastVersion ? "，預設是這個 Test Case 上次跑的那一版" : ""}）。開始 Run
        前一定會再顯示一次權限摘要並要求確認。
      </p>
      <RunHistory runs={runs} history={history} />
      <DeleteTestCase testCaseId={testCaseId} onDeleted={setDeleted} />
    </section>
  );
}

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
      <p className="note" data-role="teaching">
        逐條驗收結果在各自的 Run 頁面上。
      </p>
      {runs.isPending && <Loading what="執行歷史" />}
      <ReadFailure error={runs.error} what="執行歷史" />
      {runs.data && (
        <ListFreshness
          inFlight={history.some((run) => IN_FLIGHT_RUN_STATUSES.has(run.status))}
          updatedAt={runs.dataUpdatedAt}
          fetching={runs.isFetching && !runs.isFetchingNextPage}
          refetch={runs.refetch}
        />
      )}
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
                {(run.status_reason || run.failure_class) && (
                  <p className="note">
                    {run.status_reason ?? "未測量（伺服器沒有回報原因）"}
                    {run.failure_class && `（分類：${run.failure_class.label}）`}
                  </p>
                )}
                <p className="note">
                  {run.finished_at ? (
                    <>
                      結束於 <Timestamp at={run.finished_at} />
                    </>
                  ) : (
                    "尚未結束"
                  )}
                </p>
                <details>
                  <summary>Skill Version</summary>
                  <code>{run.skill_version_id}</code>
                </details>
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

function DeleteTestCase({
  testCaseId,
  onDeleted,
}: {
  testCaseId: string;
  onDeleted: (result: { datasets_deleted: number }) => void;
}) {
  const client = useQueryClient();
  const [error, setError] = useState<unknown>(null);

  const remove = useMutation({
    mutationFn: () => deleteTestCase(testCaseId),
    onSuccess: async (result) => {
      setError(null);
      await client.invalidateQueries({ queryKey: ["test-cases"] });
      onDeleted(result);
    },
    onError: (err) => setError(err),
  });

  return (
    <>
      <h2>刪除這個 Test Case</h2>
      <p>
        <ConfirmDelete
          scopeId={`delete-scope-${testCaseId}`}
          scope="會刪掉這個草稿與它已上傳的檔案。沒有回收桶也沒有保留期，這一頁沒有還原的地方，按下去就沒有了。已經跑過的 Run 及其快照不受影響——那是那些 Run 執行內容的紀錄。"
          pending={remove.isPending}
          onAsk={() => setError(null)}
          onConfirm={() => remove.mutate()}
          label="刪除整個 Test Case"
          confirmLabel="確認刪除整個 Test Case"
        />
      </p>
      <MutationError error={error} what="這個 Test Case" fallback="刪除沒有成功，可以再試一次。" />
    </>
  );
}

function PromptForm({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const [name, setName] = useState(testCase.name);
  const [prompt, setPrompt] = useState(testCase.user_prompt);
  const [message, setMessage] = useState("");
  const [error, setError] = useState<unknown>(null);
  const sizeReason = oversizeReason(name, prompt);

  const save = useMutation({
    mutationFn: () => updateTestCase(testCase.test_case_id, { name, user_prompt: prompt }),
    onSuccess: async () => {
      setMessage("已儲存。");
      setError(null);
      await client.invalidateQueries({ queryKey: ["test-cases"] });
    },
    onError: (err) => setError(err),
  });

  return (
    <>
      <h2>名稱與 User Prompt</h2>
      <p className="field">
        <label htmlFor="edit-name">名稱</label>
        <input
          id="edit-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          size={40}
        />{" "}
        <span className="note">名稱最多 {MAX_NAME_BYTES} bytes。</span>
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
        <br />
        <span className="note">User Prompt 最多 {MAX_PROMPT_BYTES} bytes。</span>
      </p>
      <button
        type="button"
        disabled={
          save.isPending || prompt.trim() === "" || name.trim() === "" || sizeReason !== null
        }
        aria-describedby={
          name.trim() === "" || prompt.trim() === ""
            ? "edit-required-reason"
            : sizeReason
              ? "edit-size-reason"
              : undefined
        }
        onClick={() => {
          if (sizeReason) return;
          save.mutate();
        }}
      >
        {save.isPending ? "儲存中…" : "儲存"}
      </button>{" "}
      {(name.trim() === "" || prompt.trim() === "") && (
        <span id="edit-required-reason" className="note" role="status">
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
      {sizeReason && name.trim() !== "" && prompt.trim() !== "" && (
        <span id="edit-size-reason" className="note" role="status">
          {sizeReason}
        </span>
      )}
      {message && <p role="status">{message}</p>}
      <MutationError
        error={error}
        what="名稱與 User Prompt"
        fallback="儲存沒有成功，可以再試一次。"
        serverSaysStatuses={[400]}
      />
    </>
  );
}

function CriteriaSection({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const [text, setText] = useState("");
  const [message, setMessage] = useState("");
  const [addError, setAddError] = useState<unknown>(null);
  const [suggestError, setSuggestError] = useState<unknown>(null);
  const [adoptError, setAdoptError] = useState<unknown>(null);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const refresh = () => client.invalidateQueries({ queryKey: ["test-cases"] });

  const add = useMutation({
    mutationFn: () => addCriterion(testCase.test_case_id, text),
    onSuccess: async () => {
      setText("");
      setAddError(null);
      await refresh();
    },
    onError: (err) => setAddError(err),
  });

  const suggest = useMutation({
    mutationFn: () => suggestCriteria(testCase.test_case_id),
    onSuccess: (res) => {
      setSuggestions(res.suggestions.map((s) => s.text));
      setSuggestError(null);
      setMessage(res.suggestions.length === 0 ? "這次沒有可用的建議，請自己手動輸入。" : "");
    },
    onError: (err) => setSuggestError(err),
  });

  const adopt = useMutation({
    mutationFn: (proposal: string) =>
      addCriterion(testCase.test_case_id, proposal, "suggested").then(() => proposal),
    onSuccess: async (proposal) => {
      setSuggestions((prev) => prev.filter((s) => s !== proposal));
      setAdoptError(null);
      await refresh();
    },
    onError: (err) => setAdoptError(err),
  });

  return (
    <>
      <h2>驗收條件</h2>
      <p className="note" data-role="teaching">
        每一條都會被逐項判定為通過／未通過／無法判斷。 開始 Run
        時，這一頁的內容會被凍結成快照：之後修改只影響<strong>下一次</strong>
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

      <p className="note">一個 Test Case 最多 {MAX_CRITERIA} 條驗收條件。</p>
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
          aria-describedby={text.trim() === "" ? "criterion-add-why" : undefined}
          onClick={() => add.mutate()}
        >
          {add.isPending ? "新增中…" : "新增"}
        </button>{" "}
        <button type="button" disabled={suggest.isPending} onClick={() => suggest.mutate()}>
          {suggest.isPending ? "建議中…" : "請系統建議（選用）"}
        </button>{" "}
        {text.trim() === "" && (
          <span className="note" role="status" id="criterion-add-why">
            欄位是空的
          </span>
        )}
      </p>
      <MutationError
        error={addError}
        what="這一條驗收條件"
        fallback="無法新增，可以再試一次。"
        serverSaysStatuses={[413]}
      />
      <ReadFailure error={suggestError} what="建議">
        <p role="alert">
          {suggestError instanceof ApiError && suggestError.status === 503
            ? "目前無法自動建議，驗收條件可以自己手動輸入。"
            : "無法取得建議，可以再試一次。"}
        </p>
      </ReadFailure>
      {message && <p role="status">{message}</p>}

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
                    {adopt.isPending
                      ? adopt.variables === s
                        ? "採納中…"
                        : "採納（另一項處理中…）"
                      : "採納"}
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
          <MutationError
            error={adoptError}
            what="這條建議"
            fallback="無法採納，可以再試一次。"
            serverSaysStatuses={[413]}
          />
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
  const [error, setError] = useState<unknown>(null);
  const refresh = () => client.invalidateQueries({ queryKey: ["test-cases"] });

  const mutate = useMutation({
    mutationFn: (action: "save" | "confirm" | "unconfirm" | "delete") => {
      if (action === "delete") return deleteCriterion(testCaseId, criterion.id);
      if (action === "save") return updateCriterion(testCaseId, criterion.id, { text: draft });
      return updateCriterion(testCaseId, criterion.id, { confirmed: action === "confirm" });
    },
    onSuccess: async () => {
      setError(null);
      await refresh();
    },
    onError: (err) => setError(err),
  });

  const edited = draft !== criterion.text;
  const saveReason = !edited ? "沒有變更要存" : draft.trim() === "" ? "驗收條件不能是空白" : null;

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
          aria-describedby={saveReason ? `criterion-save-${criterion.id}` : undefined}
          onClick={() => mutate.mutate("save")}
        >
          {mutate.isPending
            ? mutate.variables === "save"
              ? "儲存中…"
              : "儲存文字（另一項處理中…）"
            : "儲存文字"}
        </button>{" "}
        {criterion.confirmed_at ? (
          <button
            type="button"
            disabled={mutate.isPending}
            onClick={() => mutate.mutate("unconfirm")}
          >
            {mutate.isPending
              ? mutate.variables === "unconfirm"
                ? "取消確認中…"
                : "取消確認（另一項處理中…）"
              : "取消確認"}
          </button>
        ) : (
          <button
            type="button"
            disabled={mutate.isPending || edited}
            aria-describedby={edited ? `criterion-edited-${criterion.id}` : undefined}
            onClick={() => mutate.mutate("confirm")}
          >
            {mutate.isPending
              ? mutate.variables === "confirm"
                ? "確認中…"
                : "確認（另一項處理中…）"
              : "確認"}
          </button>
        )}{" "}
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
          onAsk={() => setError(null)}
          onConfirm={() => mutate.mutate("delete")}
          label="刪除這一條"
          confirmLabel="確認刪除這一條"
        />
      </p>
      {saveReason && (
        <p className="note" role="status" id={`criterion-save-${criterion.id}`}>
          {saveReason}
        </p>
      )}
      <MutationError error={error} what="這一條驗收條件" fallback="操作沒有成功，可以再試一次。" />
    </li>
  );
}

function RubricSection({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const stored = testCase.rubric;
  const [version, setVersion] = useState(stored?.version ?? "");
  const [items, setItems] = useState<Record<string, RubricItem>>(
    Object.fromEntries((stored?.items ?? []).map((i) => [i.id, i])),
  );
  const [message, setMessage] = useState("");
  const [error, setError] = useState<unknown>(null);

  const save = useMutation({
    mutationFn: () => {
      const list = testCase.acceptance_criteria
        .map((c) => items[c.id])
        .filter((i): i is RubricItem => i !== undefined && i.text.trim() !== "");
      // null means no rubric; {items: []} would mean a rubric with nothing in it.
      return updateTestCase(testCase.test_case_id, {
        rubric: list.length === 0 ? null : { version: version.trim(), items: list },
      });
    },
    onSuccess: async () => {
      setMessage("已儲存。");
      setError(null);
      await client.invalidateQueries({ queryKey: ["test-cases"] });
    },
    onError: (err) => setError(err),
  });

  const update = (id: string, patch: Partial<RubricItem>) =>
    setItems((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? { id, text: "", evidence_required: false }), ...patch },
    }));

  const used = testCase.acceptance_criteria.filter((c) => items[c.id]?.text.trim()).length;

  return (
    <>
      <h3>Rubric（選用）</h3>
      <p className="note" data-role="teaching">
        Rubric 是驗收條件的<strong>加強說法</strong>，不是另一套判定：每一條都掛在上面某一條驗收
        條件上，只是額外說明「做到什麼程度算過」以及「要不要引原文」。權重只是給模型看的相對
        重要性，平台不拿它算分。
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
            <span className="note" data-role="teaching">
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
            aria-describedby={
              used > 0 && version.trim() === "" ? "rubric-version-reason" : undefined
            }
            onClick={() => save.mutate()}
          >
            {save.isPending ? "儲存中…" : "儲存 Rubric"}
          </button>{" "}
          <span className="note">
            {used === 0
              ? "目前沒有任何一條有內容，儲存等於移除這個 Test Case 的 rubric。"
              : `目前 ${used} 條有內容。`}
          </span>
          {used > 0 && version.trim() === "" && (
            <span id="rubric-version-reason" className="note" role="status">
              還不能儲存，因為 Rubric 版本是空的。有內容的 rubric
              一定要有版本——評估報告要記下這次判定是在哪個版本下做的。
            </span>
          )}
          {message && <p role="status">{message}</p>}
          <MutationError
            error={error}
            what="Rubric"
            fallback="儲存沒有成功，可以再試一次。"
            serverSaysStatuses={[413]}
          />
        </>
      )}
    </>
  );
}

function DatasetSection({ testCaseId }: { testCaseId: string }) {
  const client = useQueryClient();
  const datasets = useTestCaseDatasets(testCaseId);
  const [message, setMessage] = useState("");
  const [error, setError] = useState<unknown>(null);

  const remove = useMutation({
    mutationFn: (datasetId: string) => deleteDataset(testCaseId, datasetId),
    onSuccess: async (result) => {
      setMessage(result.note);
      setError(null);
      await client.invalidateQueries({ queryKey: ["test-cases", testCaseId, "datasets"] });
    },
    onError: (err) => setError(err),
  });

  return (
    <>
      <h2>測試資料</h2>
      <p>
        <Link to="/lab/datasets" search={{ test_case: testCaseId }}>
          上傳檔案
        </Link>
      </p>
      <p className="note" data-role="teaching">
        （上傳規則會在選檔前顯示）。
      </p>
      {datasets.isPending && <Loading what="檔案清單" />}
      <ReadFailure error={datasets.error} what="檔案清單" />
      {datasets.data &&
        (datasets.data.datasets.length === 0 ? (
          <p>還沒有上傳任何檔案。</p>
        ) : (
          <>
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
                    onAsk={() => setError(null)}
                    onConfirm={() => remove.mutate(d.dataset_id)}
                    label="刪除這個檔案"
                    confirmLabel="確認刪除這個檔案"
                  />
                  <p className="note">
                    保存到 <Timestamp at={d.expires_at} /> 自動刪除
                  </p>
                </li>
              ))}
            </ul>
          </>
        ))}
      {message && <p role="status">{message}</p>}
      <MutationError error={error} what="這個檔案" fallback="刪除沒有成功，可以再試一次。" />
    </>
  );
}
