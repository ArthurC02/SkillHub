import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  createVersionFromSuggestions,
  decideSuggestion,
  setEvaluationFeedback,
  useEvaluation,
  useEvaluationRevisions,
  useRunSuggestions,
  useSuggestionDiff,
} from "../api/evaluation";
import type {
  CriterionResult,
  DeterministicFinding,
  Evaluation,
  EvidenceRef,
  ImprovementSuggestion,
  SuggestionBlockedReason,
  VersionFromSuggestions,
} from "../api/evaluation";

/**
 * 02:EVAL-001 / EVAL-002 — the evaluation report and the improvement
 * suggestions, both inside the run detail page.
 *
 * The rules this view exists to keep, none of them cosmetic:
 *
 * 1. **`succeeded` is an execution outcome, never a task verdict.** The run's
 *    terminal state is worded as execution (執行完成 / 執行失敗) and the task
 *    judgement is a separate row carrying `Evaluation.overall` (ADR-025,
 *    design §4.3). The judgement is rendered first because that is the question
 *    the user actually asked.
 * 2. **No evaluation is 「未評估」, never a pass.** The server answers 404 for a
 *    run it never judged; a blank verdict area would read as approval, so the
 *    absence is spelled out. 「評估未完成」(`status: failed`) is a third state
 *    again.
 * 3. **A model's verdict is labelled as a model's** (EVAL-001 第 5 條), and its
 *    prose is untrusted content: everything from the judge goes through React's
 *    default escaping into plain text or <pre>, never markup.
 * 4. **Expired evidence shows its excerpt and says so.** `available: false` is
 *    neither an error nor a blank: the copy kept at judgement time is displayed
 *    with the fact that the original is gone (ADR-026 決策 2).
 */

/** Execution wording for a run's terminal state. Never a pass/fail of the task. */
export const RUN_STATUS_LABEL: Record<string, string> = {
  queued: "排隊中",
  provisioning: "環境準備中",
  preparing: "準備中",
  running: "執行中",
  evaluating: "評估中",
  succeeded: "執行完成",
  failed: "執行失敗",
  cancelled: "已取消",
  timed_out: "執行逾時",
};

const OVERALL_LABEL: Record<Evaluation["overall"], string> = {
  met: "符合",
  partially_met: "部分符合",
  not_met: "未符合",
  undetermined: "無法判斷",
};

const CRITERION_LABEL: Record<CriterionResult["result"], string> = {
  passed: "通過",
  failed: "未通過",
  undetermined: "無法判斷",
};

const SOURCE_LABEL: Record<CriterionResult["source"], string> = {
  rule: "規則判定（平台自己的紀錄）",
  model: "模型評估（不是確定事實）",
  user: "使用者判定",
};

const FINDING_CATEGORY_LABEL: Record<DeterministicFinding["category"], string> = {
  spec: "規格",
  activation: "啟用",
  execution: "執行",
  effect: "任務效果",
  compatibility: "相容性",
  cost: "成本",
};

const SEVERITY_LABEL: Record<DeterministicFinding["severity"], string> = {
  error: "錯誤",
  warning: "警告",
  info: "資訊",
};

const SUGGESTION_CATEGORY_LABEL: Record<ImprovementSuggestion["category"], string> = {
  skill: "Skill 內容問題",
  runtime: "Runtime 問題",
  mcp: "MCP 問題",
  tool: "工具問題",
  dataset: "測試資料問題",
};

/** One sentence per value, so no blocked suggestion is refused without a reason. */
const BLOCKED_REASON_LABEL: Record<SuggestionBlockedReason, string> = {
  path_out_of_bounds: "建議的目標路徑指到套件外面，不能套用。",
  target_changed: "目標檔案已經和建議產生當時不同，這項建議是針對舊內容寫的，不能套用。",
  validation_blocked: "套用後套件會出現阻擋級的規格問題，不能套用。",
  access_restricted: "這個 Skill 目前處於授權受限狀態，平台不重現其套件內容，不能套用。",
  diff_unavailable: "算不出差異。看不到會改什麼就不提供套用。",
};

function usd(value: number | null): string {
  return value === null ? "未回報" : `US$${value.toFixed(4)}`;
}

/**
 * The task judgement, and the run's execution state beside it as two separate
 * statements. `runStatus` is passed in rather than fetched again: the trace
 * query already holds it and the runs table is its only authority (iron rule 5).
 */
export function EvaluationPanel({
  runId,
  runStatus,
  skillId,
}: {
  runId: string;
  runStatus?: string;
  skillId?: string;
}) {
  const [revision, setRevision] = useState<string | undefined>(undefined);
  const evaluation = useEvaluation(runId, revision);
  const revisions = useEvaluationRevisions(runId);
  const notEvaluated = evaluation.error instanceof ApiError && evaluation.error.status === 404;

  return (
    <section className="evaluation">
      <h2>任務判定</h2>

      {evaluation.isPending && <p>載入評估結果中…</p>}

      {notEvaluated && (
        <div className="notice">
          <p>
            <strong>未評估</strong>
          </p>
          <p>這個 Run 沒有評估結果。未評估不等於通過，也不等於未通過。</p>
        </div>
      )}

      {evaluation.error && !notEvaluated && (
        <p role="alert">無法讀取評估結果：{evaluation.error.message}</p>
      )}

      {evaluation.data && <EvaluationReport evaluation={evaluation.data} runId={runId} />}

      {/* Execution state, worded as execution and kept on its own row. */}
      {runStatus && (
        <p className="note">
          執行狀態：{RUN_STATUS_LABEL[runStatus] ?? runStatus}（<code>{runStatus}</code>）。
          這說的是工作負載跑完了沒有，不是任務達成了沒有。
        </p>
      )}

      {revisions.data && revisions.data.revisions.length > 1 && (
        <p>
          <label htmlFor="evaluation-revision">評估版本</label>{" "}
          <select
            id="evaluation-revision"
            value={revision ?? ""}
            onChange={(e) => setRevision(e.target.value === "" ? undefined : e.target.value)}
          >
            <option value="">目前的判定</option>
            {revisions.data.revisions.map((r) => (
              <option key={r.evaluation_id} value={r.evaluation_id}>
                {r.evaluated_at}｜{OVERALL_LABEL[r.overall]}｜prompt {r.judge_prompt_version}
                {r.rubric_version ? `｜rubric ${r.rubric_version}` : ""}
                {r.superseded_at ? "（已被取代）" : ""}
              </option>
            ))}
          </select>
        </p>
      )}

      {evaluation.data && evaluation.data.status === "completed" && (
        <SuggestionsPanel runId={runId} skillId={skillId} />
      )}
    </section>
  );
}

function EvaluationReport({ evaluation, runId }: { evaluation: Evaluation; runId: string }) {
  const superseded = Boolean(evaluation.superseded_at);

  return (
    <div>
      {superseded && (
        <p className="notice">
          你正在看歷史判定，它已於 {evaluation.superseded_at} 被較新的評估取代。
        </p>
      )}

      {evaluation.status === "failed" && (
        <p className="notice">
          <strong>評估未完成</strong>：這次判定沒有跑完（例如模型閘道不可用或證據讀不到）。
          這與「未評估」不同，也不會被當成通過。
        </p>
      )}
      {evaluation.status === "pending" && <p className="notice">評估進行中，以下結果尚未定案。</p>}

      <p className={`verdict verdict-${evaluation.overall}`}>
        任務判定：<strong>{OVERALL_LABEL[evaluation.overall]}</strong>
      </p>
      {evaluation.summary && <p>{evaluation.summary}</p>}

      {!evaluation.evidence_complete && (
        <p className="notice">
          判定所依據的材料不完整（Trace 有缺漏、Artifact 讀不到，或輸入被截斷）。
          在這個前提下，逐條判定不會記為通過。
        </p>
      )}

      <h3>逐條驗收條件</h3>
      {evaluation.criterion_results.length === 0 ? (
        <p>這次評估沒有逐條結果。</p>
      ) : (
        <ul className="criterion-list">
          {evaluation.criterion_results.map((c) => (
            <li key={c.criterion_id} className={`criterion criterion-${c.result}`}>
              <p>
                <span className={`badge badge-criterion-${c.result}`}>
                  {CRITERION_LABEL[c.result]}
                </span>{" "}
                {c.text}
              </p>
              <p className="note">判定來源：{SOURCE_LABEL[c.source]}</p>
              {/* Untrusted when source is `model`: plain text, never markup. */}
              {c.reason && <p>{c.reason}</p>}
              <EvidenceList evidence={c.evidence} />
            </li>
          ))}
        </ul>
      )}

      <h3>這個 Run 的問題（六類）</h3>
      {evaluation.deterministic_findings.length === 0 ? (
        <p>沒有列出問題。這不等於一切正常，只表示這些檢查沒有產生發現。</p>
      ) : (
        <ul className="finding-list">
          {evaluation.deterministic_findings.map((f, i) => (
            <li key={`${f.category}-${i}`}>
              <p>
                <span className="badge">{FINDING_CATEGORY_LABEL[f.category]}</span>{" "}
                <span className={`badge badge-severity-${f.severity}`}>
                  {SEVERITY_LABEL[f.severity]}
                </span>{" "}
                {f.message}
              </p>
              <EvidenceList evidence={f.evidence} />
            </li>
          ))}
        </ul>
      )}

      <h3>評估本身的成本</h3>
      <p>
        {usd(evaluation.cost.evaluation_usd)}
        {evaluation.cost.source === "estimated" ? "（估算值）" : "（模型閘道實付）"}
      </p>
      <p className="note">
        {evaluation.cost.note}
        {" 這是平台判定花的錢，與 Run 自己花的錢分開列，不相加。"}
      </p>

      <h3>判定資訊</h3>
      <ul className="note">
        <li>Judge 模型：{evaluation.judge_model || "未使用模型"}</li>
        <li>Judge prompt 版本：{evaluation.judge_prompt_version}</li>
        <li>Rubric 版本：{evaluation.rubric_version ?? "無 rubric（不是採用預設 rubric）"}</li>
        <li>評估時間：{evaluation.evaluated_at}</li>
      </ul>

      <FeedbackForm runId={runId} evaluation={evaluation} disabled={superseded} />
    </div>
  );
}

/**
 * ADR-026 決策 2. An expired citation keeps the excerpt taken when the verdict
 * was made and says the original is gone — not a blank, and not a pretence that
 * the trace event is still there.
 */
function EvidenceList({ evidence }: { evidence: EvidenceRef[] }) {
  if (evidence.length === 0) return <p className="note">沒有附上證據引用。</p>;
  return (
    <ul className="evidence-list">
      {evidence.map((e, i) => (
        <li key={`${e.kind}-${i}`}>
          <p className="note">
            {e.kind === "trace_event" && `Trace 事件 ${e.trace_event_id ?? ""}`}
            {e.kind === "artifact" &&
              `Artifact ${e.artifact_path ?? ""}${
                e.byte_range ? `（位元組 ${e.byte_range.start}–${e.byte_range.end}）` : ""
              }`}
            {e.kind === "agent_output" &&
              `Agent 輸出${
                e.char_range ? `（字元 ${e.char_range.start}–${e.char_range.end}）` : ""
              }`}
          </p>
          {!e.available && (
            <p className="note">原始資料已過期或已刪除，以下是評估當時保存的摘要。</p>
          )}
          {/* Crossed the trust boundary: inert text, never interpreted. */}
          <pre>{e.excerpt}</pre>
          {e.excerpt_truncated && <p className="note">（摘要已截斷，不是全文）</p>}
        </li>
      ))}
    </ul>
  );
}

function FeedbackForm({
  runId,
  evaluation,
  disabled,
}: {
  runId: string;
  evaluation: Evaluation;
  disabled: boolean;
}) {
  const client = useQueryClient();
  const [comment, setComment] = useState(evaluation.feedback?.comment ?? "");
  const [message, setMessage] = useState("");

  const submit = useMutation({
    mutationFn: (helpful: boolean) => setEvaluationFeedback(runId, helpful, comment),
    onSuccess: async () => {
      setMessage("已送出回饋。");
      await client.invalidateQueries({ queryKey: ["evaluation", runId] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "回饋送出失敗。"),
  });

  if (disabled) {
    return <p className="note">回饋只能對目前的判定填寫。</p>;
  }

  return (
    <div className="evaluation-feedback">
      <h3>這份評估有幫助嗎</h3>
      {evaluation.feedback && (
        <p className="note">
          你先前的回答：{evaluation.feedback.helpful ? "有幫助" : "沒幫助"}（
          {evaluation.feedback.submitted_at}）。可以改。
        </p>
      )}
      <label htmlFor="feedback-comment">補充說明（選填）</label>
      <textarea
        id="feedback-comment"
        rows={3}
        maxLength={2000}
        value={comment}
        onChange={(e) => setComment(e.target.value)}
      />
      <p>
        <button type="button" disabled={submit.isPending} onClick={() => submit.mutate(true)}>
          有幫助
        </button>{" "}
        <button type="button" disabled={submit.isPending} onClick={() => submit.mutate(false)}>
          沒幫助
        </button>
      </p>
      {message && <p role="status">{message}</p>}
    </div>
  );
}

/** 02:EVAL-002 — the suggestions, their diffs, the per-item decision, and apply. */
function SuggestionsPanel({ runId, skillId }: { runId: string; skillId?: string }) {
  const client = useQueryClient();
  const suggestions = useRunSuggestions(runId);
  const [applied, setApplied] = useState<VersionFromSuggestions | null>(null);
  const [message, setMessage] = useState("");

  const apply = useMutation({
    mutationFn: (ids: string[]) =>
      createVersionFromSuggestions(skillId as string, suggestions.data?.evaluation_id ?? "", ids),
    onSuccess: async (result) => {
      setApplied(result);
      setMessage("");
      await client.invalidateQueries({ queryKey: ["suggestions", runId] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "建立新版本失敗。"),
  });

  const notFound = suggestions.error instanceof ApiError && suggestions.error.status === 404;
  if (suggestions.isPending) return <p>載入改善建議中…</p>;
  if (notFound) return null;
  if (suggestions.error) {
    return <p role="alert">無法讀取改善建議：{suggestions.error.message}</p>;
  }

  const accepted = suggestions.data.suggestions.filter((s) => s.decision === "accepted");

  return (
    <section className="suggestions">
      <h2>改善建議</h2>
      {suggestions.data.suggestions.length === 0 ? (
        <p>這份評估沒有產生改善建議。</p>
      ) : (
        <ul className="suggestion-list">
          {suggestions.data.suggestions.map((s) => (
            <SuggestionItem key={s.suggestion_id} suggestion={s} runId={runId} />
          ))}
        </ul>
      )}

      {suggestions.data.suggestions.length > 0 && (
        <div className="suggestion-apply">
          <p className="note">
            採納建議會建立一個<strong>新的 Skill Version</strong>
            ，不會覆寫已經跑過的版本；新版本的套件內容不同，開始 Run 前必須重新確認權限摘要。
          </p>
          {skillId ? (
            <button
              type="button"
              disabled={accepted.length === 0 || apply.isPending}
              onClick={() => apply.mutate(accepted.map((s) => s.suggestion_id))}
            >
              以已接受的 {accepted.length} 項建議建立新版本
            </button>
          ) : (
            <p role="alert">
              這個頁面需要 <code>?skill=</code> 才能建立新版本：Run 的讀取端點目前不回傳 skill
              id，無從得知要把新版本建在哪個 Skill 底下。
            </p>
          )}
        </div>
      )}

      {message && <p role="alert">{message}</p>}
      {applied && <AppliedResult result={applied} skillId={skillId} />}
    </section>
  );
}

function SuggestionItem({
  suggestion,
  runId,
}: {
  suggestion: ImprovementSuggestion;
  runId: string;
}) {
  const client = useQueryClient();
  const [showDiff, setShowDiff] = useState(false);
  const [message, setMessage] = useState("");

  const decide = useMutation({
    mutationFn: (decision: "accepted" | "rejected") =>
      decideSuggestion(suggestion.suggestion_id, decision),
    onSuccess: async () => {
      setMessage("");
      await client.invalidateQueries({ queryKey: ["suggestions", runId] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "無法記錄決定。"),
  });

  return (
    <li className="suggestion">
      <p>
        <span className="badge">{SUGGESTION_CATEGORY_LABEL[suggestion.category]}</span>{" "}
        <code>{suggestion.target_path}</code>
      </p>
      {/* Model-written text. Escaped, never markup. */}
      <p>{suggestion.problem}</p>
      <p className="note">預期影響（模型的預測，不是量測結果）：{suggestion.expected_impact}</p>
      <EvidenceList evidence={suggestion.evidence} />

      <p>
        <button type="button" onClick={() => setShowDiff((v) => !v)}>
          {showDiff ? "收起差異" : "查看差異"}
        </button>{" "}
        <button
          type="button"
          aria-pressed={suggestion.decision === "accepted"}
          disabled={decide.isPending}
          onClick={() => decide.mutate("accepted")}
        >
          接受
        </button>{" "}
        <button
          type="button"
          aria-pressed={suggestion.decision === "rejected"}
          disabled={decide.isPending}
          onClick={() => decide.mutate("rejected")}
        >
          拒絕
        </button>{" "}
        <span className="note">
          目前：
          {suggestion.decision === "accepted"
            ? "已接受"
            : suggestion.decision === "rejected"
              ? "已拒絕"
              : "尚未決定"}
          {suggestion.applied_skill_version_id ? "（已套用於新版本）" : ""}
        </span>
      </p>
      {message && <p role="alert">{message}</p>}
      {showDiff && <SuggestionDiffView suggestionId={suggestion.suggestion_id} />}
    </li>
  );
}

function SuggestionDiffView({ suggestionId }: { suggestionId: string }) {
  const diff = useSuggestionDiff(suggestionId, true);
  if (diff.isPending) return <p>載入差異中…</p>;
  if (diff.error) return <p role="alert">無法讀取差異：{diff.error.message}</p>;

  return (
    <div>
      {!diff.data.applicable && (
        <p className="notice">
          目前無法套用：
          {diff.data.blocked_reason
            ? BLOCKED_REASON_LABEL[diff.data.blocked_reason]
            : "伺服器沒有給原因。"}
        </p>
      )}
      {diff.data.unified_diff ? (
        <pre className="diff">{diff.data.unified_diff}</pre>
      ) : (
        <p>沒有可顯示的差異。</p>
      )}
    </div>
  );
}

function AppliedResult({ result, skillId }: { result: VersionFromSuggestions; skillId?: string }) {
  return (
    <div className="notice">
      <p>
        已建立新版本 <strong>#{result.version_number}</strong>
        {result.duplicate ? "（內容與既有版本相同，沿用該版本）" : ""}
      </p>
      <p className="note">
        內容雜湊 <code>{result.content_hash}</code>；套用了 {result.applied_suggestion_ids.length}{" "}
        項建議。
      </p>
      {result.rejected_suggestions.length > 0 && (
        <>
          <p>以下建議沒有被套用：</p>
          <ul>
            {result.rejected_suggestions.map((r) => (
              <li key={r.suggestion_id}>
                {r.message}（{BLOCKED_REASON_LABEL[r.blocked_reason]}）
              </li>
            ))}
          </ul>
        </>
      )}
      {skillId && (
        <p>
          <Link to="/skills/$skillId" params={{ skillId }}>
            前往新版本所在的 Skill
          </Link>
        </p>
      )}
    </div>
  );
}
