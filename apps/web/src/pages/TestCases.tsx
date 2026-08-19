import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  addCriterion,
  createTestCase,
  deleteCriterion,
  deleteDataset,
  suggestCriteria,
  updateCriterion,
  updateTestCase,
  useOwnSkills,
  useTestCase,
  useTestCaseDatasets,
  useTestCases,
} from "../api/testcases";
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
  if (c.confirmed_at) return `已確認（${c.confirmed_at}）`;
  return c.source === "suggested" ? "系統建議，尚未確認" : "尚未確認";
}

export function TestCaseList() {
  const navigate = useNavigate();
  const testCases = useTestCases();
  const skills = useOwnSkills();
  const [skillId, setSkillId] = useState("");
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [message, setMessage] = useState("");
  const rows = testCases.data?.pages.flatMap((page) => page.test_cases) ?? [];

  const create = useMutation({
    mutationFn: () => createTestCase(skillId, name, prompt),
    onSuccess: (tc) =>
      navigate({ to: "/lab/test-cases/$testCaseId", params: { testCaseId: tc.test_case_id } }),
    onError: (err) => setMessage(err instanceof Error ? err.message : "無法建立 Test Case。"),
  });

  return (
    <section className="page">
      <h1>Test Case</h1>
      <p className="note">
        Test Case 是可編輯的草稿：User Prompt、測試資料與驗收條件。開始 Run 時，平台會把當下的
        Prompt 與驗收條件凍結成快照，之後再改都不會動到已經跑過的 Run。
      </p>

      <h2>建立新的 Test Case</h2>
      {skills.error && <p role="alert">無法讀取你的 Skill 清單：{skills.error.message}</p>}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <p>
          <label htmlFor="tc-skill">Skill</label>{" "}
          <select id="tc-skill" value={skillId} onChange={(e) => setSkillId(e.target.value)}>
            <option value="">請選擇</option>
            {skills.data?.skills.map((s) => (
              <option key={s.skill_id} value={s.skill_id}>
                {s.name}
              </option>
            ))}
          </select>
        </p>
        <p>
          <label htmlFor="tc-name">名稱</label>{" "}
          <input id="tc-name" value={name} onChange={(e) => setName(e.target.value)} size={40} />
        </p>
        <p>
          <label htmlFor="tc-prompt">User Prompt</label>
          <br />
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

      <h2>既有的 Test Case</h2>
      {testCases.isPending && <p>載入中…</p>}
      {testCases.error && <p role="alert">無法讀取 Test Case：{testCases.error.message}</p>}
      {testCases.data &&
        (rows.length === 0 ? (
          <p>還沒有 Test Case。</p>
        ) : (
          <ul className="search-results">
            {rows.map((tc) => (
              <li key={tc.test_case_id}>
                <Link to="/lab/test-cases/$testCaseId" params={{ testCaseId: tc.test_case_id }}>
                  {tc.name}
                </Link>
                <p className="note">
                  驗收條件 {tc.acceptance_criteria.length} 條（已確認{" "}
                  {tc.acceptance_criteria.filter((c) => c.confirmed_at).length} 條）· 最後修改{" "}
                  {tc.updated_at}
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
    </section>
  );
}

export function TestCaseDetail() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId" });
  const testCase = useTestCase(testCaseId);

  if (testCase.isPending) return <p>載入 Test Case 中…</p>;
  if (testCase.error) {
    const missing = testCase.error instanceof ApiError && testCase.error.status === 404;
    return (
      <p role="alert">
        {missing ? "找不到這個 Test Case。" : `無法讀取 Test Case：${testCase.error.message}`}
      </p>
    );
  }

  return (
    <section className="page" key={testCaseId}>
      <h1>{testCase.data.name}</h1>
      <p className="note">
        <Link to="/lab/test-cases">回到 Test Case 列表</Link>
      </p>
      <PromptForm testCase={testCase.data} />
      <CriteriaSection testCase={testCase.data} />
      <RubricSection testCase={testCase.data} />
      <DatasetSection testCaseId={testCaseId} />
      <h2>開始試跑</h2>
      <p className="note">
        <Link
          to="/lab/run"
          search={{ skill: testCase.data.skill_id, test_case: testCaseId, version: undefined }}
        >
          前往執行前權限確認
        </Link>
        （還需要填入要執行的 Skill Version id）。開始 Run 前一定會再顯示一次權限摘要並要求確認。
      </p>
    </section>
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
      <p>
        <label htmlFor="edit-name">名稱</label>{" "}
        <input id="edit-name" value={name} onChange={(e) => setName(e.target.value)} size={40} />
      </p>
      <p>
        <label htmlFor="edit-prompt">User Prompt</label>
        <br />
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
      </button>
      {message && <p role="status">{message}</p>}
    </>
  );
}

function CriteriaSection({ testCase }: { testCase: TestCase }) {
  const client = useQueryClient();
  const [text, setText] = useState("");
  const [message, setMessage] = useState("");
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
    onSuccess: async () => {
      setMessage("");
      await refresh();
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
        </button>
      </p>
      {message && <p role="alert">{message}</p>}
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
      {edited && criterion.confirmed_at && (
        <p className="note">改動文字後儲存會清除這一條的確認，因為當初確認的是舊的文字。</p>
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
            onClick={() => mutate.mutate("confirm")}
          >
            確認
          </button>
        )}{" "}
        <button type="button" disabled={mutate.isPending} onClick={() => mutate.mutate("delete")}>
          刪除
        </button>
      </p>
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
      <h2>Rubric（選用）</h2>
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
      {datasets.isPending && <p>載入檔案清單中…</p>}
      {datasets.error && <p role="alert">無法讀取檔案清單：{datasets.error.message}</p>}
      {datasets.data &&
        (datasets.data.datasets.length === 0 ? (
          <p>還沒有上傳任何檔案。</p>
        ) : (
          <ul className="file-tree">
            {datasets.data.datasets.map((d) => (
              <li key={d.dataset_id}>
                {d.file_name} <span className="note">（{d.content_type}）</span>{" "}
                <button
                  type="button"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(d.dataset_id)}
                >
                  刪除
                </button>
              </li>
            ))}
          </ul>
        ))}
      {message && <p role="status">{message}</p>}
    </>
  );
}
