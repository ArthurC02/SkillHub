import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  actOnCreationSession,
  createCreationSession,
  getCreationLimits,
  getCreationSession,
  listCreationSessions,
  type CreationAction,
  type CreationSession as Session,
  type CreationSnapshot,
  type CreationState,
} from "../api/creation";
import type { CategorizedFindings, ImportFinding } from "../api/import";
import { ReadFailure } from "./LoginRequired";
import { ReferencePicker } from "./GenerateSkill";
import { Findings } from "./Findings";
import { Timestamp } from "./Timestamp";
const labels: Record<CreationState, string> = {
  queued: "等待處理",
  working: "正在創作",
  waiting_input: "等待你的補充",
  waiting_confirmation: "等待你確認",
  draft_ready: "草稿可供檢查",
  candidate_ready: "候選版本已建立",
  saved: "已保存",
  cancelled: "已取消",
  failed: "這一步未完成",
  needs_reupload: "請重新上傳流程圖",
};
type Extra = Omit<CreationAction, "command_id" | "expected_revision" | "kind">;
type Mode = "message" | "diagram" | "references";
function readImage(file: File): Promise<{ media_type: string; data: string }> {
  if (
    !["image/png", "image/jpeg", "image/webp"].includes(file.type) ||
    file.size === 0 ||
    file.size > 4_000_000 // one-number: creationMaxDiagramBytes
  )
    return Promise.reject(
      new Error("請選擇最多 4,000,000 位元組（約 3.8 MB）以內的 PNG、JPEG 或 WebP 流程圖。"),
    );
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("流程圖無法讀取，請重新選擇。"));
    reader.onload = () =>
      resolve({ media_type: file.type, data: String(reader.result).split(",")[1] });
    reader.readAsDataURL(file);
  });
}
/**
 * apps/platform's `skillpkg.Report` (creator/creation's `ValidateCreationDraft`
 * marshals it verbatim) is `{findings: Finding[], blocked, ...}` — a flat
 * array, not import's pre-grouped `{errors,warnings,infos}`. `Finding` itself
 * is field-for-field `ImportFinding` (severity/code/path/message/details), so
 * group by severity and reuse the SAME renderer failed imports use
 * (Findings.tsx) instead of a second component that flattens every finding
 * into identical bullets.
 */
function groupDraftFindings(raw: string): CategorizedFindings | undefined {
  try {
    const parsed = JSON.parse(raw) as { findings?: unknown };
    if (!Array.isArray(parsed.findings)) return undefined;
    const groups: CategorizedFindings = { errors: [], warnings: [], infos: [] };
    for (const item of parsed.findings as ImportFinding[]) {
      if (item.severity === "error") groups.errors.push(item);
      else if (item.severity === "warning") groups.warnings.push(item);
      else if (item.severity === "info") groups.infos.push(item);
    }
    return groups;
  } catch {
    return undefined;
  }
}
function DraftFindings({ raw }: { raw: string }) {
  const grouped = groupDraftFindings(raw);
  return grouped ? <Findings findings={grouped} level={4} /> : <p>{raw}</p>;
}
type RunObservation = {
  execution_status: string;
  evaluation?: { evaluation_available: boolean; overall?: string };
};
/** The newest `tool` message reporting on this run (creation.go's `attach_run`
 * appends one such message per confirmation; a later confirmation can attach
 * a newer run under the same candidate). */
function findRunObservation(
  messages: CreationSnapshot["messages"],
  runID: string,
): RunObservation | undefined {
  let found: RunObservation | undefined;
  for (const m of messages) {
    if (m.role !== "tool") continue;
    try {
      const parsed = JSON.parse(m.content) as RunObservation & { run_id?: string };
      if (parsed.run_id === runID) found = parsed;
    } catch {
      // Not every tool message is a run observation.
    }
  }
  return found;
}
type DiagramUnderstanding = {
  nodes: string[];
  conditions: string[];
  branches: string[];
  uncertainties: string[];
};
const diagramSections: (keyof DiagramUnderstanding)[] = [
  "nodes",
  "conditions",
  "branches",
  "uncertainties",
];
const diagramLabels: Record<keyof DiagramUnderstanding, string> = {
  nodes: "節點",
  conditions: "條件",
  branches: "分支",
  uncertainties: "不確定處",
};
function parseDiagramUnderstanding(raw: string): DiagramUnderstanding | undefined {
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
    const record = value as Record<string, unknown>;
    if (Object.keys(record).length !== diagramSections.length) return undefined;
    if (!diagramSections.every((key) => Array.isArray(record[key]))) return undefined;
    const sections = Object.fromEntries(
      diagramSections.map((key) => [key, record[key] as unknown[]]),
    ) as Record<keyof DiagramUnderstanding, unknown[]>;
    if (sections.nodes.length === 0) return undefined;
    if (!diagramSections.every((key) => sections[key].every((item) => typeof item === "string")))
      return undefined;
    return Object.fromEntries(
      diagramSections.map((key) => [key, sections[key] as string[]]),
    ) as DiagramUnderstanding;
  } catch {
    return undefined;
  }
}
function DiagramUnderstandingView({ raw }: { raw: string }) {
  const structured = parseDiagramUnderstanding(raw);
  if (!structured)
    return (
      <>
        <p>{raw}</p>
        <p className="note">請在對話要求重新整理，或重新上傳後確認。</p>
      </>
    );
  return (
    <div>
      {diagramSections.map((key) => (
        <section key={key}>
          <h5>{diagramLabels[key]}</h5>
          {structured[key].length > 0 ? (
            <ul>
              {structured[key].map((item, i) => (
                <li key={i}>{item}</li>
              ))}
            </ul>
          ) : (
            <p>未列出</p>
          )}
        </section>
      ))}
    </div>
  );
}
function declaredReferenceField(value?: string) {
  return value?.trim() ? value : "未宣告";
}
export function CreationSession() {
  const client = useQueryClient();
  const [id, setID] = useState(""),
    [mode, setMode] = useState<Mode>("message"),
    [message, setMessage] = useState(""),
    [budget, setBudget] = useState(""),
    [file, setFile] = useState<File>(),
    [refs, setRefs] = useState<{ id: string; name: string }[]>([]),
    [runID, setRunID] = useState(""),
    [raiseBudget, setRaiseBudget] = useState(""),
    [raiseError, setRaiseError] = useState(""),
    [error, setError] = useState<unknown>(),
    [busy, setBusy] = useState(false);
  const pending = useRef<{ key: string; body: CreationAction } | undefined>(undefined);
  const startPending = useRef<
    { key: string; body: { id: string; message: string; budget_usd: number } } | undefined
  >(undefined);
  const sessions = useQuery({
    queryKey: ["creation-sessions"],
    queryFn: listCreationSessions,
    retry: false,
  });
  const limits = useQuery({
    queryKey: ["creation-limits"],
    queryFn: getCreationLimits,
    retry: false,
  });
  const current = useQuery({
    queryKey: ["creation-session", id],
    queryFn: () => getCreationSession(id),
    enabled: !!id,
    retry: false,
    refetchInterval: (q) =>
      ["queued", "working"].includes(q.state.data?.state ?? "") ? 1000 : false,
  });
  const session = current.data,
    p = session?.snapshot;
  const run = p?.candidate?.run_id ? findRunObservation(p.messages, p.candidate.run_id) : undefined;
  const runNotPassing =
    !!run && (run.execution_status !== "succeeded" || run.evaluation?.overall !== "met");
  const terminal = !!session && ["saved", "cancelled"].includes(session.state);
  const working = !!session && ["queued", "working"].includes(session.state);
  const locked = busy || working || terminal;
  const save = (value: Session) => {
    setID(value.id);
    client.setQueryData<Session>(["creation-session", value.id], (old) =>
      old && old.revision > value.revision ? old : value,
    );
    void client.invalidateQueries({ queryKey: ["creation-sessions"] });
  };
  const send = async (value: Session, kind: CreationAction["kind"], extra: Extra = {}) => {
    const key = JSON.stringify([value.id, kind, extra]);
    if (pending.current?.key !== key)
      pending.current = {
        key,
        body: {
          command_id: crypto.randomUUID(),
          expected_revision: value.revision,
          kind,
          ...extra,
        },
      };
    try {
      const next = await actOnCreationSession(value.id, pending.current.body);
      pending.current = undefined;
      save(next);
      return next;
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        pending.current = undefined;
        await client.invalidateQueries({ queryKey: ["creation-session", value.id] });
      }
      throw err;
    }
  };
  const perform = async (kind: CreationAction["kind"], extra: Extra = {}) => {
    if (!session) return;
    setBusy(true);
    setError(undefined);
    try {
      await send(session, kind, extra);
      if (kind === "message") setMessage("");
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  const submitRaiseBudget = async () => {
    if (!p || !limits.data) return;
    const amount = Number(raiseBudget);
    if (!Number.isFinite(amount) || amount <= p.budget_usd || amount > limits.data.max_budget_usd) {
      setRaiseError(
        `請填寫高於目前上限 $${p.budget_usd} 且不超過 $${limits.data.max_budget_usd} 的金額。`,
      );
      return;
    }
    setRaiseError("");
    await perform("raise_budget", { budget_usd: amount });
  };
  const submit = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const diagram = mode === "diagram" ? (file ? await readImage(file) : undefined) : undefined;
      if (mode === "diagram" && !diagram) throw new Error("請先選擇流程圖。");
      if (mode === "references" && refs.length === 0) throw new Error("請先選擇一個參考 Skill。");
      if (mode === "message" && !message.trim()) throw new Error("請描述想完成的任務。");
      let value = session;
      if (!value) {
        const amount = Number(budget);
        if (!Number.isFinite(amount) || amount <= 0)
          throw new Error("請填寫這次同意支付的美元預算上限。");
        if (
          limits.data &&
          (amount < limits.data.min_budget_usd || amount > limits.data.max_budget_usd)
        )
          throw new Error(
            `請填寫介於 $${limits.data.min_budget_usd} 與 $${limits.data.max_budget_usd} 之間的預算上限。`,
          );
        const initial = mode === "message" ? message : "";
        const key = JSON.stringify([initial, amount]);
        if (startPending.current?.key !== key)
          startPending.current = {
            key,
            body: { id: crypto.randomUUID(), message: initial, budget_usd: amount },
          };
        value = await createCreationSession(startPending.current.body);
        save(value);
        startPending.current = undefined;
        if (mode === "message") {
          setMessage("");
          return;
        }
      }
      if (mode === "message") {
        await send(value, "message", { message });
        setMessage("");
      }
      if (mode === "diagram") {
        await send(value, "diagram", { diagram });
        setFile(undefined);
      }
      if (mode === "references")
        await send(value, "select_references", { reference_skill_ids: refs.map((r) => r.id) });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div>
      <h3>和 Agent 一起創作 Skill</h3>
      <p className="note">
        逐步確認需求與草稿，保存到私人工作區。模型處理與改善會使用這次核准的預算。
      </p>
      <ReadFailure error={sessions.error ?? current.error} what="創作紀錄" />
      {sessions.data && sessions.data.length > 0 && (
        <label>
          恢復創作
          <select
            aria-label="恢復創作"
            value={id}
            disabled={busy}
            onChange={(e) => {
              setID(e.target.value);
              setMessage("");
              setFile(undefined);
              setRefs([]);
              pending.current = undefined;
              setError(undefined);
            }}
          >
            <option value="">開始新的創作</option>
            {sessions.data.slice(0, 50).map((s) => (
              <option key={s.id} value={s.id}>
                {s.snapshot.brief.slice(0, 40) || "尚未確認需求"} · {labels[s.state]}
              </option>
            ))}
          </select>
        </label>
      )}
      {session && <p role="status">創作狀態：{labels[session.state]}</p>}
      {session && !terminal && (
        <p>
          這次創作可進行到 <Timestamp at={session.deadline} />
          ；紀錄保留到 <Timestamp at={session.expires_at} />。
        </p>
      )}
      {working && (
        <p className="note">
          可以關掉這一頁；回來時從「恢復創作」繼續。上次更新{" "}
          {/* `current` polls every 1s while queued/working (refetchInterval
              above), the same cadence InFlight.tsx uses to justify its own
              `relative` Timestamp — see InFlight.tsx. */}
          <Timestamp at={session.updated_at} relative />
        </p>
      )}
      {p ? (
        <>
          <p>
            預算上限 $ {p.budget_usd}；已知費用{" "}
            {p.spent_usd === undefined ? "未知" : "$ " + p.spent_usd}；仍占用預算 $ {p.reserved_usd}
            。{p.usage_unknown && "部分用量未能取得，費用仍是未知，不能當作零。"}
          </p>
          {limits.data && (
            <p>
              已用 {p.steps}／{limits.data.max_steps} 步、工具 {p.tool_calls}／
              {limits.data.max_tool_calls} 次
            </p>
          )}
          {limits.data &&
            (session?.state === "failed" ||
              p.budget_usd - (p.spent_usd ?? 0) - p.reserved_usd <
                2 * limits.data.min_budget_usd) && (
              <div>
                <label>
                  提高這次預算上限（美元）
                  <input
                    aria-label="提高這次預算上限（美元）"
                    inputMode="decimal"
                    disabled={busy}
                    value={raiseBudget}
                    onChange={(e) => setRaiseBudget(e.target.value)}
                  />
                </label>
                <button type="button" disabled={busy} onClick={() => void submitRaiseBudget()}>
                  提高預算後繼續
                </button>
                {!!raiseError && <p role="alert">{raiseError}</p>}
              </div>
            )}
        </>
      ) : (
        <label>
          這次預算上限（美元）
          {limits.data && (
            <span className="note">
              （介於 $ {limits.data.min_budget_usd} 與 $ {limits.data.max_budget_usd} 之間）
            </span>
          )}
          <input
            aria-label="這次預算上限（美元）"
            inputMode="decimal"
            value={budget}
            onChange={(e) => setBudget(e.target.value)}
          />
        </label>
      )}
      {!terminal && (
        <>
          <fieldset disabled={locked}>
            <legend>提供創作素材</legend>
            {(
              [
                ["message", "自然語言"],
                ["diagram", "流程圖"],
                ["references", "目錄參考"],
              ] as const
            ).map(([key, label]) => (
              <label key={key}>
                <input
                  type="radio"
                  name="creation-mode"
                  checked={mode === key}
                  onChange={() => setMode(key)}
                />
                {label}
              </label>
            ))}
          </fieldset>
          {mode === "message" && (
            <label>
              想完成的任務
              <textarea
                aria-label="想完成的任務"
                maxLength={4000}
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                disabled={busy}
              />
            </label>
          )}
          {mode === "diagram" && (
            <label>
              流程圖（PNG、JPEG、WebP；最多 4,000,000 位元組，約 3.8 MB）
              <input
                aria-label="流程圖"
                type="file"
                accept="image/png,image/jpeg,image/webp"
                disabled={locked}
                onChange={(e) => setFile(e.target.files?.[0])}
              />
            </label>
          )}
          {mode === "references" && (
            <ReferencePicker
              disabled={locked}
              references={refs}
              onToggle={(skillID, name) =>
                setRefs((old) =>
                  old.some((r) => r.id === skillID)
                    ? old.filter((r) => r.id !== skillID)
                    : old.length < 3
                      ? [...old, { id: skillID, name }]
                      : old,
                )
              }
            />
          )}
          <button type="button" disabled={locked} onClick={() => void submit()}>
            {busy ? "送出中…" : session ? "送出素材" : "開始互動創作"}
          </button>
          {working && <p className="note">正在處理目前素材；完成後即可補充或確認。</p>}
        </>
      )}
      {!!error && (
        <ReadFailure error={error} what="互動創作">
          <p role="alert">
            {error instanceof ApiError && error.status === 409
              ? "進度已更新，輸入仍保留。請檢查最新內容後再送出。"
              : error instanceof TypeError
                ? "網路連線失敗，請重試。"
                : error instanceof Error
                  ? error.message
                  : "這一步未完成，請重試。"}
          </p>
        </ReadFailure>
      )}
      {p && (
        <>
          <ol>
            {p.messages.map((m, i) => (
              <li key={i}>
                <strong>{{ user: "你", assistant: "Agent", tool: "工具結果" }[m.role]}：</strong>
                {m.content}
              </li>
            ))}
          </ol>
          {p.brief && (
            <section>
              <h4>需求摘要</h4>
              <p>{p.brief}</p>
              <h5>驗收條件</h5>
              {p.acceptance_criteria.length > 0 ? (
                <ol>
                  {p.acceptance_criteria.map((c, i) => (
                    <li key={i}>{c}</li>
                  ))}
                </ol>
              ) : (
                <p>模型尚未提出驗收條件</p>
              )}
              <p>{p.brief_confirmed ? "需求摘要與驗收條件皆已確認" : "尚未確認"}</p>
              {p.pending_action === "confirm_brief" && (
                <button disabled={locked} onClick={() => void perform("confirm_brief")}>
                  確認需求摘要與驗收條件
                </button>
              )}
            </section>
          )}
          {p.diagram_understanding && (
            <section>
              <h4>流程圖理解</h4>
              <DiagramUnderstandingView raw={p.diagram_understanding} />
              <p>{p.diagram_confirmed ? "已確認" : "尚未確認"}</p>
              {p.pending_action === "confirm_diagram" && (
                <button
                  disabled={locked || !parseDiagramUnderstanding(p.diagram_understanding)}
                  onClick={() => void perform("confirm_diagram")}
                >
                  確認流程圖理解
                </button>
              )}
            </section>
          )}
          {(p.references.length > 0 || p.pending_action === "confirm_references") && (
            <section>
              <h4>參考 Skill</h4>
              <table>
                <thead>
                  <tr>
                    <th>Skill</th>
                    <th>摘要</th>
                    <th>相容</th>
                    <th>工具</th>
                    <th>版本</th>
                    <th>狀態</th>
                  </tr>
                </thead>
                <tbody>
                  {p.references.map((r) => (
                    <tr key={r.skill_id}>
                      <th scope="row">{r.name}</th>
                      <td>{declaredReferenceField(r.description)}</td>
                      <td>{declaredReferenceField(r.compatibility)}</td>
                      <td>{declaredReferenceField(r.allowed_tools)}</td>
                      <td>
                        <details>
                          <summary>固定版本</summary>
                          {r.version_id}
                        </details>
                      </td>
                      <td>{!r.available ? "目前不可用" : r.confirmed ? "已確認" : "尚未確認"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {p.pending_action === "confirm_references" && (
                <button
                  disabled={locked || p.references.some((r) => !r.available)}
                  onClick={() => void perform("confirm_references")}
                >
                  確認參考 Skill
                </button>
              )}
            </section>
          )}
          {p.draft && (
            <section>
              <h4>Skill 草稿：{p.draft.skill.name}</h4>
              <p>{p.draft.skill.description}</p>
              <p>
                允許工具：{p.draft.skill.allowed_tools || "未宣告"}。相容條件：
                {p.draft.skill.compatibility || "未宣告"}。
              </p>
              {p.previous_draft && (
                <details>
                  <summary>比較上一份草稿（revision {p.previous_draft.revision}）</summary>
                  <pre>{p.previous_draft.skill.body}</pre>
                  {p.previous_draft.skill.files.map((f) => (
                    <pre key={f.path}>{f.path + "\n" + f.content}</pre>
                  ))}
                </details>
              )}
              <pre>{p.draft.skill.body}</pre>
              {p.draft.skill.files.map((f) => (
                <details key={f.path}>
                  <summary>{f.path}</summary>
                  <pre>{f.content}</pre>
                </details>
              ))}
              <DraftFindings raw={p.draft.validation} />
              <p>
                {p.draft.blocked
                  ? "靜態檢查阻擋保存，請補充需求後修訂。"
                  : "已完成靜態檢查；這不代表試跑成功。"}
              </p>
              {!p.candidate && (
                <button
                  disabled={locked || p.draft.blocked || !p.draft.content_hash}
                  onClick={() =>
                    void perform("materialize", { content_hash: p.draft!.content_hash })
                  }
                >
                  建立私人候選版本
                </button>
              )}
              {p.candidate && (
                <>
                  <p>
                    <Link
                      to="/lab/run"
                      search={{
                        skill: p.candidate.skill_id,
                        version: p.candidate.version_id,
                        test_case: p.candidate.test_case_id,
                      }}
                    >
                      檢查權限與費用後試跑此版本
                    </Link>
                  </p>
                  {p.candidate.test_case_id && <p>已依確認的驗收條件建立 Test Case</p>}
                  {p.candidate.run_id ? (
                    <>
                      <Link to="/runs/$runId" params={{ runId: p.candidate.run_id }}>
                        查看這次 Run 結果
                      </Link>
                      {run && (
                        <p>
                          試跑結果：{run.execution_status}；評估：
                          {run.evaluation?.overall ?? "無評估"}
                        </p>
                      )}
                    </>
                  ) : (
                    <p>目前尚未連結試跑結果。</p>
                  )}
                  {!terminal && (
                    <>
                      <label>
                        用來改善的 Run ID
                        <input
                          aria-label="用來改善的 Run ID"
                          value={runID}
                          onChange={(e) => setRunID(e.target.value)}
                        />
                      </label>
                      <button
                        disabled={locked || !runID.trim()}
                        onClick={() => void perform("attach_run", { run_id: runID.trim() })}
                      >
                        參考這次 Run 繼續改善
                      </button>
                    </>
                  )}
                </>
              )}
              {!terminal && (
                <>
                  <p>
                    保存將採用目前顯示的草稿與版本。{!p.candidate?.run_id && "這份草稿尚未試跑。"}
                    {runNotPassing && "試跑未通過或未評估；保存前請確認。"}
                  </p>
                  <button
                    disabled={locked || p.draft.blocked || !p.draft.content_hash}
                    onClick={() =>
                      void perform("finalize", { content_hash: p.draft!.content_hash })
                    }
                  >
                    確認保存到私人工作區
                  </button>
                </>
              )}
              {session?.state === "saved" && p.candidate && (
                <Link to="/skills/$skillId" params={{ skillId: p.candidate.skill_id }}>
                  開啟已保存的 Skill
                </Link>
              )}
            </section>
          )}
          {!terminal && (
            <button disabled={busy} onClick={() => void perform("cancel")}>
              取消這次創作
            </button>
          )}
        </>
      )}
    </div>
  );
}
