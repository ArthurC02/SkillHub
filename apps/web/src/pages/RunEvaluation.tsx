import { Loading } from "../components/Loading";
import { StateIcon, type IconState } from "../components/StateIcon";
import { Timestamp, formatAt } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import {
  EVALUATION_POLL_MAX_404,
  EVALUATION_POLL_MAX_PENDING,
  createVersionFromSuggestions,
  decideSuggestion,
  setEvaluationFeedback,
  useEvaluation,
  useEvaluationRevisions,
  useRunSuggestions,
  useSuggestionDiff,
} from "../api/evaluation";
import { useRun } from "../api/runs";
import type { RunStatus } from "../api/trace";
import { Reveal } from "../components/Reveal";
import type {
  CriterionResult,
  DeterministicFinding,
  Evaluation,
  EvidenceMatch,
  EvidenceRef,
  ImprovementSuggestion,
  RejectedSuggestion,
  SuggestionBlockedReason,
  VersionFromSuggestions,
} from "../api/evaluation";

export const RUN_STATUS_LABEL: Record<RunStatus, string> = {
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

export function runStatusLabel(status: string): string {
  return RUN_STATUS_LABEL[status as RunStatus] ?? status;
}

export const OVERALL_LABEL: Record<Evaluation["overall"], string> = {
  met: "符合",
  partially_met: "部分符合",
  not_met: "未符合",
  undetermined: "無法判斷",
};

export const CRITERION_LABEL: Record<CriterionResult["result"], string> = {
  passed: "通過",
  failed: "未通過",
  undetermined: "無法判斷",
};

const EVIDENCE_UNVERIFIABLE_PREFIX = "evidence_unverifiable: ";

function isEvidenceUnverifiable(c: CriterionResult): boolean {
  return c.result === "undetermined" && c.reason.startsWith(EVIDENCE_UNVERIFIABLE_PREFIX);
}

export const SOURCE_LABEL: Record<CriterionResult["source"], string> = {
  rule: "規則判定（平台自己的紀錄）",
  model: "模型評估（不是確定事實）",
  user: "使用者判定",
};

export const FINDING_CATEGORY_LABEL: Record<DeterministicFinding["category"], string> = {
  spec: "規格",
  activation: "啟用",
  execution: "執行",
  effect: "任務效果",
  compatibility: "相容性",
  cost: "成本",
};

export const SEVERITY_LABEL: Record<DeterministicFinding["severity"], string> = {
  error: "錯誤",
  warning: "警告",
  info: "資訊",
};

export const SUGGESTION_CATEGORY_LABEL: Record<ImprovementSuggestion["category"], string> = {
  skill: "Skill 內容問題",
  runtime: "Runtime 問題",
  mcp: "MCP 問題",
  tool: "工具問題",
  dataset: "測試資料問題",
};

export const BLOCKED_REASON_LABEL: Record<SuggestionBlockedReason, string> = {
  path_out_of_bounds: "建議的目標路徑指到套件外面，不能套用。",
  target_changed: "目標檔案已經和建議產生當時不同，這項建議是針對舊內容寫的，不能套用。",
  validation_blocked: "套用後套件會出現阻擋級的規格問題，不能套用。",
  access_restricted: "這個 Skill 目前處於授權受限狀態，平台不重現其套件內容，不能套用。",
  diff_unavailable: "算不出差異。看不到會改什麼就不提供套用。",
};

function credits(value: number | null): string {
  return value === null ? "未測量" : `${value} 點`;
}

export function EvaluationPanel({ runId, runStatus }: { runId: string; runStatus?: string }) {
  const { evaluation: revision } = useSearch({ strict: false }) as { evaluation?: string };
  const navigate = useNavigate();
  const seenEvaluationId = useRef<string | undefined>(undefined);
  useEffect(() => {
    seenEvaluationId.current = undefined;
  }, [runId]);
  const awaiting = runStatus === "succeeded" || runStatus === "failed";
  const evaluation = useEvaluation(runId, revision, awaiting);
  const revisions = useEvaluationRevisions(runId);
  const notEvaluated = evaluation.error instanceof ApiError && evaluation.error.status === 404;
  const stoppedAsking = notEvaluated && evaluation.errorUpdateCount >= EVALUATION_POLL_MAX_404;

  const client = useQueryClient();
  const currentEvaluationId = revision ? undefined : evaluation.data?.evaluation_id;
  // Only invalidates on a changed id, not on every mount — StrictMode's double
  // effect run sees the same id the second time, so this stays a no-op then.
  useEffect(() => {
    if (!currentEvaluationId) return;
    if (seenEvaluationId.current && seenEvaluationId.current !== currentEvaluationId) {
      void client.invalidateQueries({ queryKey: ["evaluation", runId, "revisions"] });
    }
    seenEvaluationId.current = currentEvaluationId;
  }, [client, runId, currentEvaluationId]);
  const evaluating = !revision && evaluation.data?.status === "pending";

  return (
    <section>
      <h2>任務判定</h2>

      {evaluation.isPending && !evaluating && <Loading what="評估結果" />}

      {evaluating && (
        <div className="notice" role="status">
          <p>
            <strong>評估進行中</strong>
            {evaluation.pendingPollStopped
              ? "——這一筆評估說自己還在做，但這一頁已經停止再查了。"
              : "——判定還在做。它會自己完成，不需要你回來按任何東西。"}
          </p>
          <p className="note">可以關掉這一頁（平台在跑，不是你的瀏覽器）</p>
          <p className="note">
            這一段沒有進度可以報——評審不是分批完成的，它要嘛給出判定要嘛失敗，
            而兩種結果都會出現在這裡。
            {evaluation.pendingPollStopped ? (
              <>
                這一頁查了 {EVALUATION_POLL_MAX_PENDING} 次、約{" "}
                {(EVALUATION_POLL_MAX_PENDING * 3) / 60}{" "}
                分鐘，狀態都還是「進行中」，所以它不會再自己更新了。
                停下來的是這一頁的查詢，不是那個 job：重新整理這一頁會再查一次。
                一直停在這裡代表那個工作沒有在推進，而不是判定為未通過。
              </>
            ) : (
              "這一頁每 3 秒自己查一次。"
            )}
          </p>
        </div>
      )}

      {notEvaluated && !evaluating && (
        <div className="notice">
          <p>
            <strong>未評估</strong>
          </p>
          <p>這個 Run 沒有評估結果。未評估不等於通過，也不等於未通過。</p>
          {awaiting && !revision && !stoppedAsking && (
            <p className="note">
              這一頁每 3 秒再查一次；如果有評估正在排隊，結果會自己出現在這裡，不必重新整理。
            </p>
          )}
          {awaiting && !revision && stoppedAsking && (
            <p className="note">
              這一頁已經停止再查了——查了 {EVALUATION_POLL_MAX_404}{" "}
              次都還是沒有評估，所以它不會再自己更新。 這通常表示沒有人替這個 Run
              送出評估，而不是評估失敗。重新整理這一頁會再查一次。
            </p>
          )}
        </div>
      )}

      {!notEvaluated && <ReadFailure error={evaluation.error} what="評估結果" />}

      {evaluation.data && !evaluating && (
        <EvaluationReport evaluation={evaluation.data} runStatus={runStatus} />
      )}

      {!evaluation.data && runStatus && <ExecutionState runStatus={runStatus} />}

      {revisions.data && revisions.data.revisions.length > 1 && (
        <p>
          <label htmlFor="evaluation-revision">評估版本</label>{" "}
          <select
            id="evaluation-revision"
            value={revision ?? ""}
            onChange={(e) =>
              void navigate({
                to: "/runs/$runId",
                params: { runId },
                search: (prev) => ({
                  ...prev,
                  evaluation: e.target.value === "" ? undefined : e.target.value,
                }),
              })
            }
          >
            <option value="">目前的判定</option>
            {revisions.data.revisions.map((r) => (
              <option key={r.evaluation_id} value={r.evaluation_id}>
                {formatAt(r.evaluated_at)}｜{OVERALL_LABEL[r.overall]}｜prompt{" "}
                {r.judge_prompt_version}
                {r.rubric_version ? `｜rubric ${r.rubric_version}` : ""}
                {r.superseded_at ? "（已被取代）" : ""}
              </option>
            ))}
          </select>
        </p>
      )}

      {evaluation.data && evaluation.data.status === "completed" && (
        <SuggestionsPanel runId={runId} />
      )}

      {evaluation.data && (
        <FeedbackForm
          runId={runId}
          evaluation={evaluation.data}
          disabled={Boolean(evaluation.data.superseded_at)}
        />
      )}
    </section>
  );
}

function ExecutionState({ runStatus }: { runStatus: string }) {
  return (
    <p className="note">
      執行狀態：{runStatusLabel(runStatus)}（<code>{runStatus}</code>）。
      這說的是工作負載跑完了沒有，不是任務達成了沒有。
    </p>
  );
}

function EvaluationReport({
  evaluation,
  runStatus,
}: {
  evaluation: Evaluation;
  runStatus?: string;
}) {
  return (
    <div>
      {evaluation.superseded_at && (
        <p className="notice">
          你正在看歷史判定，它已於 <Timestamp at={evaluation.superseded_at} /> 被較新的評估取代。
        </p>
      )}

      {evaluation.status === "failed" && (
        <p className="notice">
          <strong>評估未完成</strong>：這次判定沒有跑完（例如模型閘道不可用或證據讀不到）。
          這與「未評估」不同，也不會被當成通過。
        </p>
      )}
      {evaluation.status === "pending" && <p className="notice">評估進行中，以下結果尚未定案。</p>}

      <p className="verdict">
        任務判定：<strong>{OVERALL_LABEL[evaluation.overall]}</strong>
      </p>
      {runStatus && <ExecutionState runStatus={runStatus} />}
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
        <CriterionSection results={evaluation.criterion_results} />
      )}

      <h3>這個 Run 的問題（六類）</h3>
      {evaluation.deterministic_findings.length === 0 ? (
        <p>沒有列出問題。這不等於一切正常，只表示這些檢查沒有產生發現。</p>
      ) : (
        <>
          <MatchLegend evidence={evaluation.deterministic_findings.flatMap((f) => f.evidence)} />
          <ul className="finding-list">
            {evaluation.deterministic_findings.map((f, i) => (
              <li className="criterion" key={`${f.category}-${i}`}>
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
        </>
      )}

      <h3>評估本身用掉的點數</h3>
      <p>
        {credits(evaluation.cost.evaluation_credits)}
        {evaluation.cost.source === "gateway" && "（模型閘道實付）"}
        {evaluation.cost.source === "estimated" && "（估算值）"}
      </p>
      <p className="note">
        {evaluation.cost.note}
        {" 這是平台判定用掉的點數，與 Run 自己用掉的分開列，不相加。"}
      </p>

      <details>
        <summary>判定資訊（Judge 模型與版本）</summary>
        <ul className="note">
          {evaluation.judge_model ? (
            <>
              <li>Judge 模型：{evaluation.judge_model}</li>
              <li>Judge prompt 版本：{evaluation.judge_prompt_version}</li>
            </>
          ) : (
            <li>
              Judge：這次沒有跑（沒有驗收條件，或平台沒有設定 Judge），所以沒有模型與 prompt
              版本可記
            </li>
          )}
          <li>Rubric 版本：{evaluation.rubric_version ?? "無 rubric（不是採用預設 rubric）"}</li>
          <li>
            評估時間：
            <Timestamp at={evaluation.evaluated_at} />
          </li>
        </ul>
      </details>
    </div>
  );
}

// Ties keep the first source seen: strict `>` plus Map's insertion order.
function listSource(results: CriterionResult[]): CriterionResult["source"] | undefined {
  const tally = new Map<CriterionResult["source"], number>();
  for (const c of results) {
    if (isEvidenceUnverifiable(c)) continue;
    tally.set(c.source, (tally.get(c.source) ?? 0) + 1);
  }
  let best: CriterionResult["source"] | undefined;
  for (const [source, n] of tally) if (n > (best ? (tally.get(best) ?? 0) : 1)) best = source;
  return best;
}

function CriterionSection({ results }: { results: CriterionResult[] }) {
  const shared = listSource(results);
  return (
    <>
      {shared && <p className="note">判定來源：{SOURCE_LABEL[shared]}；不同的會在該條標出。</p>}
      <MatchLegend evidence={results.flatMap((c) => c.evidence)} />
      <ul className="criterion-list">
        {results.map((c) => (
          <CriterionItem key={c.criterion_id} criterion={c} listSource={shared} />
        ))}
      </ul>
    </>
  );
}

function CriterionItem({
  criterion: c,
  listSource: shared,
}: {
  criterion: CriterionResult;
  listSource?: CriterionResult["source"];
}) {
  const downgraded = isEvidenceUnverifiable(c);
  const criterionIconState: IconState = downgraded
    ? "degraded"
    : c.result === "passed"
      ? "pass"
      : c.result === "failed"
        ? "fail"
        : "unknown";

  return (
    <li className={`criterion criterion-${c.result}${downgraded ? " criterion-unverifiable" : ""}`}>
      <p>
        <span
          className={`badge badge-criterion-${c.result}${
            downgraded ? " badge-criterion-unverifiable" : ""
          }`}
        >
          <StateIcon state={criterionIconState} />
          {downgraded ? "證據無法回驗" : CRITERION_LABEL[c.result]}
        </span>{" "}
        {c.text}
      </p>
      {downgraded ? (
        <p className="note">
          判定來源：平台降級（模型原本有結論，但它引用的證據在平台資料裡對不上，因此不採信）。
          <strong>這不是「模型自己說不知道」</strong>
          ——那一種會顯示為「無法判斷」。這一條要查的是引用為什麼回驗不過，不是模型有沒有把握。
        </p>
      ) : (
        c.source !== shared && <p className="note">判定來源：{SOURCE_LABEL[c.source]}</p>
      )}
      {c.reason && <p>{c.reason}</p>}
      <EvidenceList evidence={c.evidence} />
    </li>
  );
}

export type MatchKey = EvidenceMatch | "unrecorded";

export const MATCH_NOTE: Record<MatchKey, string> = {
  exact: "引文已逐字回驗。",
  normalized:
    "引文已回驗——需要正規化後才比對得上（全形半形、空白、頭尾標點）。原文與引用有細微差異，內容相同。",
  not_found: "這段引文在本次 Run 的可回驗來源裡找不到，因此不作為證據。",
  not_checked:
    "只證明這個檔案存在（路徑、大小、雜湊都在 manifest 上），沒有回驗任何引文——平台不會打開產物內容。",
  unrecorded: "這份報告產生時還沒有記錄引文回驗結果，無法判斷這段引文是否被回驗過。",
};

export const MATCH_WORD: Record<MatchKey, string> = {
  exact: "已逐字回驗",
  normalized: "正規化後比對",
  not_found: "找不到",
  not_checked: "未回驗引文",
  unrecorded: "回驗結果未記錄",
};

const MATCH_BADGE: Record<MatchKey, string> = {
  exact: "badge",
  normalized: "badge",
  not_found: "badge badge-danger",
  not_checked: "badge badge-unverified",
  unrecorded: "badge badge-unverified",
};

const MATCH_ICON: Record<MatchKey, IconState> = {
  exact: "pass",
  normalized: "pass",
  not_found: "fail",
  not_checked: "unknown",
  unrecorded: "unknown",
};

const MATCH_ORDER: MatchKey[] = ["exact", "normalized", "not_found", "not_checked", "unrecorded"];

function matchKey(e: EvidenceRef): MatchKey {
  return e.match ?? "unrecorded";
}

function MatchLegend({ evidence }: { evidence: EvidenceRef[] }) {
  const kinds = MATCH_ORDER.filter((k) => evidence.some((e) => matchKey(e) === k));
  if (kinds.length === 0) return null;
  return (
    <ul className="note">
      {kinds.map((k) => (
        <li key={k}>
          <span className={MATCH_BADGE[k]}>
            <StateIcon state={MATCH_ICON[k]} />
            {MATCH_WORD[k]}
          </span>{" "}
          {MATCH_NOTE[k]}
        </li>
      ))}
    </ul>
  );
}

export const KIND_WORD: Record<EvidenceRef["kind"], string> = {
  trace_event: "Trace 事件",
  artifact: "Artifact",
  agent_output: "Agent 輸出",
};

function EvidenceList({ evidence }: { evidence: EvidenceRef[] }) {
  if (evidence.length === 0) return <p className="note">沒有附上證據引用。</p>;
  return (
    <ul className="evidence-list">
      {evidence.map((e, i) => (
        <li key={`${e.kind}-${i}`}>
          <p className="note">
            <span className={MATCH_BADGE[matchKey(e)]}>
              <StateIcon state={MATCH_ICON[matchKey(e)]} />
              {MATCH_WORD[matchKey(e)]}
            </span>{" "}
            {e.kind === "trace_event" && "Trace 事件"}
            {e.kind === "artifact" &&
              `Artifact ${e.artifact_path ?? ""}${
                e.byte_range ? `（位元組 ${e.byte_range.start}–${e.byte_range.end}）` : ""
              }`}
            {e.kind === "agent_output" &&
              `Agent 輸出${
                e.char_range ? `（字元 ${e.char_range.start}–${e.char_range.end}）` : ""
              }`}
          </p>
          {e.reattributed_from && (
            <p className="note">
              Judge 原本標為「{KIND_WORD[e.reattributed_from]}」，實際出處是「{KIND_WORD[e.kind]}
              」， 已更正。標錯來源與捏造引文是兩件不同的事，這一筆是前者。
            </p>
          )}
          {e.kind === "trace_event" && (
            <details>
              <summary>事件 ID</summary>
              {e.trace_event_id ? (
                <code>{e.trace_event_id}</code>
              ) : (
                <p className="note">這筆引用沒有附上事件 ID。</p>
              )}
            </details>
          )}
          {!e.available && (
            <p className="note">原始資料已過期或已刪除，以下是評估當時保存的摘要。</p>
          )}
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
  const [error, setError] = useState<unknown>(null);

  const submit = useMutation({
    mutationFn: (helpful: boolean) => setEvaluationFeedback(runId, helpful, comment),
    onSuccess: async () => {
      setMessage("已送出回饋。");
      setError(null);
      await client.invalidateQueries({ queryKey: ["evaluation", runId] });
    },
    onError: (err) => setError(err),
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
          <Timestamp at={evaluation.feedback.submitted_at} />
          ）。可以改。
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
      <ReadFailure error={error} what="回饋">
        <p role="alert">
          {error instanceof ApiError && error.status === 404
            ? "這個 Run 目前沒有可以附回饋的判定。"
            : "回饋沒有送出，可以再按一次。"}
        </p>
      </ReadFailure>
    </div>
  );
}

function SuggestionsPanel({ runId }: { runId: string }) {
  const client = useQueryClient();
  const suggestions = useRunSuggestions(runId);
  const run = useRun(runId).data;
  const skillId = run?.skill_id;
  const [applied, setApplied] = useState<VersionFromSuggestions | null>(null);
  const [error, setError] = useState<unknown>(null);

  const apply = useMutation({
    mutationFn: (ids: string[]) =>
      createVersionFromSuggestions(skillId as string, suggestions.data?.evaluation_id ?? "", ids),
    onSuccess: async (result) => {
      setApplied(result);
      setError(null);
      await client.invalidateQueries({ queryKey: ["suggestions", runId] });
    },
    onError: (err) => setError(err),
  });

  const notFound = suggestions.error instanceof ApiError && suggestions.error.status === 404;
  if (suggestions.isPending) return <Loading what="改善建議" />;
  if (notFound) return null;
  if (suggestions.error) {
    return <ReadFailure error={suggestions.error} what="改善建議" />;
  }

  const accepted = suggestions.data.suggestions.filter((s) => s.decision === "accepted");

  return (
    <section>
      <h3>改善建議</h3>
      {suggestions.data.suggestions.length === 0 ? (
        <p>這份評估沒有產生改善建議。</p>
      ) : (
        <>
          <p className="note">「預期影響」是模型的預測，不是量測結果。</p>
          <MatchLegend evidence={suggestions.data.suggestions.flatMap((s) => s.evidence)} />
          <ul className="suggestion-list">
            {suggestions.data.suggestions.map((s) => (
              <SuggestionItem key={s.suggestion_id} suggestion={s} runId={runId} />
            ))}
          </ul>
        </>
      )}

      {suggestions.data.suggestions.length > 0 && (
        <div>
          <p className="note">
            採納建議會建立一個<strong>新的 Skill Version</strong>
            ，不會覆寫已經跑過的版本；新版本的套件內容不同，開始 Run 前必須重新確認權限摘要。
          </p>
          <button
            type="button"
            disabled={!skillId || accepted.length === 0 || apply.isPending}
            onClick={() => apply.mutate(accepted.map((s) => s.suggestion_id))}
          >
            以已接受的 {accepted.length} 項建議建立新版本
          </button>
          {!skillId && <p className="note">正在讀取這個 Run 屬於哪個 Skill…</p>}
        </div>
      )}

      <ReadFailure error={error} what="建立新版本">
        <div role="alert">
          <ApplyFailureBody error={error} />
        </div>
      </ReadFailure>
      {applied && <AppliedResult result={applied} testCaseId={run?.test_case_id} />}
    </section>
  );
}

function ApplyFailureBody({ error }: { error: unknown }) {
  if (!(error instanceof ApiError)) return <p>套用失敗，可以再按一次。</p>;
  if (error.status === 422) {
    const body = error.body as { rejected_suggestions?: RejectedSuggestion[] } | undefined;
    const rejected = body?.rejected_suggestions ?? [];
    return (
      <>
        <p>沒有一項建議可以套用，所以沒有建立新版本。</p>
        {rejected.length > 0 && (
          <ul>
            {rejected.map((r) => (
              <li key={r.suggestion_id}>
                {r.message}（{BLOCKED_REASON_LABEL[r.blocked_reason]}）
              </li>
            ))}
          </ul>
        )}
      </>
    );
  }
  if (error.status === 500) {
    return (
      <p>套用失敗。如果版本清單裡已經出現新版本，它可以用，只是「由哪些建議產生」的紀錄沒寫成。</p>
    );
  }
  return <p>套用失敗，可以再按一次。</p>;
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
  const [error, setError] = useState<unknown>(null);

  const decide = useMutation({
    mutationFn: (decision: "accepted" | "rejected") =>
      decideSuggestion(suggestion.suggestion_id, decision),
    onSuccess: async () => {
      setError(null);
      await client.invalidateQueries({ queryKey: ["suggestions", runId] });
    },
    onError: (err) => setError(err),
  });

  return (
    <li className="suggestion">
      <p>
        <span className="badge">{SUGGESTION_CATEGORY_LABEL[suggestion.category]}</span>{" "}
        <code>{suggestion.target_path}</code>
      </p>
      <p>{suggestion.problem}</p>
      <p className="note">預期影響：{suggestion.expected_impact}</p>
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
          disabled={decide.isPending || Boolean(suggestion.applied_skill_version_id)}
          aria-describedby={
            suggestion.applied_skill_version_id
              ? `reject-note-${suggestion.suggestion_id}`
              : undefined
          }
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
        {suggestion.applied_skill_version_id && (
          <span className="note" id={`reject-note-${suggestion.suggestion_id}`}>
            已建成版本的建議不能撤回
          </span>
        )}
      </p>
      <ReadFailure error={error} what="決定">
        <p role="alert">這個決定沒有記錄，可以再按一次。</p>
      </ReadFailure>
      {showDiff && <SuggestionDiffView suggestionId={suggestion.suggestion_id} />}
    </li>
  );
}

function SuggestionDiffView({ suggestionId }: { suggestionId: string }) {
  const diff = useSuggestionDiff(suggestionId, true);
  if (diff.isPending) return <Loading what="差異" />;
  if (diff.error) return <ReadFailure error={diff.error} what="差異" />;

  return (
    <div>
      {!diff.data.applicable && (
        <p className="notice notice-danger">
          目前無法套用：
          {diff.data.blocked_reason
            ? BLOCKED_REASON_LABEL[diff.data.blocked_reason]
            : "伺服器沒有給原因。"}
        </p>
      )}
      {diff.data.unified_diff ? (
        <pre className="diff">
          <Reveal text={diff.data.unified_diff} />
        </pre>
      ) : (
        <p>沒有可顯示的差異。</p>
      )}
    </div>
  );
}

function AppliedResult({
  result,
  testCaseId,
}: {
  result: VersionFromSuggestions;
  testCaseId?: string;
}) {
  return (
    <div role="status">
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
      {testCaseId ? (
        <p>
          <Link
            to="/lab/run"
            search={{
              skill: result.skill_id,
              version: result.version_id,
              test_case: testCaseId,
            }}
          >
            以新版本重跑這個 Test Case
          </Link>
          ：連過去的是執行前權限確認畫面，仍須在那裡確認一次才會開始 Run。
        </p>
      ) : (
        <p className="note">
          這個 Run 的 Test Case 草稿已不存在，無法從這裡以相同輸入重跑新版本；新版本本身不受影響。
        </p>
      )}
      <p>
        <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
          前往新版本所在的 Skill
        </Link>
      </p>
    </div>
  );
}
