import { Loading } from "../components/Loading";
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
import type {
  CriterionResult,
  DeterministicFinding,
  Evaluation,
  EvidenceMatch,
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

/**
 * Execution wording for a run's terminal state. Never a pass/fail of the task.
 *
 * `Record<RunStatus, …>` and no longer `Record<string, string>`: keyed on the
 * union, a value added to the contract stops this file compiling instead of
 * printing its English enum into a Chinese sentence (02:NFR-007 第 3 條 — the
 * failure `WorkspaceRuns.tsx`'s header already records happening once).
 * `runStatusLabel` below is what the seven `string`-typed call sites use, so the
 * fallback lives in one place rather than being re-typed as `?? x` at each.
 */
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

/** The label for a status the compiler only knows as a `string`. */
export function runStatusLabel(status: string): string {
  return RUN_STATUS_LABEL[status as RunStatus] ?? status;
}

/**
 * Exported for `RunCompare.tsx`, which used to keep its own `Record<string,
 * string>` copy of both maps. Keyed on the union, so a new server enum value is
 * a compile error on both screens instead of a silent `undefined` on one
 * (02:NFR-001 — the two planes must not word the same fact two ways).
 */
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

/**
 * 04 丙-10. Defence 3 downgrades a criterion to `undetermined` when the judge's
 * citations do not resolve against the platform's own data, and marks it by
 * prefixing the reason (apps/platform/internal/trial/improvement/judge.go `merge`). Two
 * very different things share the verdict `undetermined` without this: 「引用回驗
 * 失敗，平台不採信一個有結論的判定」 and 「模型自己說不確定」. The failure this
 * distinction exists to catch has already happened once — EVAL-013 v1 found 45
 * correct verdicts thrown away over a prefix in the quoted text, and on screen
 * that was indistinguishable from a judge with no opinion.
 */
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

/** One sentence per value, so no blocked suggestion is refused without a reason. */
export const BLOCKED_REASON_LABEL: Record<SuggestionBlockedReason, string> = {
  path_out_of_bounds: "建議的目標路徑指到套件外面，不能套用。",
  target_changed: "目標檔案已經和建議產生當時不同，這項建議是針對舊內容寫的，不能套用。",
  validation_blocked: "套用後套件會出現阻擋級的規格問題，不能套用。",
  access_restricted: "這個 Skill 目前處於授權受限狀態，平台不重現其套件內容，不能套用。",
  diff_unavailable: "算不出差異。看不到會改什麼就不提供套用。",
};

function usd(value: number | null): string {
  // 設計 §2.9 的表列詞;閘道沒有回報一個成本，不是 0。
  return value === null ? "未測量" : `US$${value.toFixed(4)}`;
}

/**
 * The task judgement, and the run's execution state beside it as two separate
 * statements. `runStatus` is passed in rather than fetched again: the trace
 * query already holds it and the runs table is its only authority (iron rule 5).
 */
export function EvaluationPanel({ runId, runStatus }: { runId: string; runStatus?: string }) {
  /*
   * 資訊架構 §0.1 R4. Which evaluation you are reading is 「你在看哪一份東西」,
   * not a reading preference: the revisions are immutable `evaluation_id`s
   * (ADR-003 / ADR-026), and a superseded verdict is a different verdict from
   * 目前的判定. Held in component state, it could not be linked, could not
   * survive a reload, and a colleague opening the address arrived on whatever
   * the current judgement happens to be — for a re-evaluated run, not the one
   * under discussion. IA §4 only ever examined this page's mode toggle; this
   * control was never looked at.
   */
  const { evaluation: revision } = useSearch({ strict: false }) as { evaluation?: string };
  const navigate = useNavigate();
  const seenEvaluationId = useRef<string | undefined>(undefined);
  useEffect(() => {
    seenEvaluationId.current = undefined;
  }, [runId]);
  // Hoisted out of the call because the banner below needs it too: it is what
  // separates 「沒有人評過這個 Run」 from 「評審正在跑，這一頁在等它」.
  const awaiting = runStatus === "succeeded" || runStatus === "failed";
  const evaluation = useEvaluation(runId, revision, awaiting);
  const revisions = useEvaluationRevisions(runId);
  const notEvaluated = evaluation.error instanceof ApiError && evaluation.error.status === 404;
  // 404 polling gave up (api/evaluation.ts). The sentence below has to change
  // with it, or the page goes on promising a re-check it no longer performs.
  const stoppedAsking = notEvaluated && evaluation.errorUpdateCount >= EVALUATION_POLL_MAX_404;

  /*
   * The revision list follows the verdict. `useEvaluation` polls, this one does
   * not and nothing invalidated it, so a re-evaluation landing while the page was
   * open swapped the report's content with no switcher to say which of the two
   * you were reading — ADR-026 / 設計 §2.5 require them to be distinguishable, and
   * until a reload they were not.
   *
   * Invalidating on a *changed* id rather than on every arrival: firing on mount
   * too would spend a duplicate request on every run page for nothing. The ref
   * survives StrictMode's double effect because the second pass sees the same id.
   */
  const client = useQueryClient();
  const currentEvaluationId = revision ? undefined : evaluation.data?.evaluation_id;
  useEffect(() => {
    if (!currentEvaluationId) return;
    if (seenEvaluationId.current && seenEvaluationId.current !== currentEvaluationId) {
      void client.invalidateQueries({ queryKey: ["evaluation", runId, "revisions"] });
    }
    seenEvaluationId.current = currentEvaluationId;
  }, [client, runId, currentEvaluationId]);
  /*
   * 設計 §2.12, and the one place in the app that still got it backwards.
   *
   * A `pending` evaluation is a row that says **the judge is running right now**,
   * and this panel used to hand it to `EvaluationReport` like any other answer.
   * 進行中 is a third axis, not a value of the verdict.
   *
   * Deliberately **not** including the 404 case, although `useEvaluation` polls
   * that too (api/evaluation.ts). A 404 says only that no evaluation exists —
   * true of a judge that has not started, and equally true of an old run nobody
   * ever evaluated. Rendering 「評估進行中」 for the second one would be a promise
   * with nothing enforcing it, which is the §2.2 shape this file argues against
   * everywhere else. 未評估 stays the answer there, with the polling stated as
   * what it is.
   */
  const evaluating = !revision && evaluation.data?.status === "pending";

  return (
    <section>
      <h2>任務判定</h2>

      {evaluation.isPending && !evaluating && <Loading what="評估結果" />}

      {evaluating && (
        <div className="notice" role="status">
          {/*
            The pending poll is bounded too (api/evaluation.ts), and when it runs
            out this sentence has to go with it: 「它會自己完成」 is a promise about
            a worker this page has stopped watching, and a judge that died leaves
            the row saying `pending` for as long as anyone cares to look.
          */}
          <p>
            <strong>評估進行中</strong>
            {evaluation.pendingPollStopped
              ? "——這一筆評估說自己還在做，但這一頁已經停止再查了。"
              : "——判定還在做。它會自己完成，不需要你回來按任何東西。"}
          </p>
          {/*
            §2.12 第 2 條的第三件事（能不能離開），and 設計 §2.13: the SENTENCE
            stays flat, the DERIVATION does not. 「評估是平台佇列裡的一個工作
            （evaluate_run），由 worker 執行，瀏覽器不在那條路徑上」 is D 類 —
            a reader who has seen it once does not press anything different the
            second time — and `components/InFlight.tsx` carries the same
            reasoning about the same queue eighty characters longer, on the same
            page while a run is executing. The two now say one thing in one
            wording; the Tip that the derivation belongs in does not exist yet
            (§1.3 第四種機制, 丙-142 第二批).
          */}
          <p className="note">可以關掉這一頁（平台在跑，不是你的瀏覽器）</p>
          {/*
            §2.12 asks for a changing quantity, and this one honestly has none:
            a judge either returns a verdict or fails, and there is no
            intermediate count to report. Saying so beats inventing a progress
            bar out of elapsed time, which would move whether or not anything
            was happening.
          */}
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
          {/*
            §2.12's 「會不會自己結束」, sized to what is actually known: the page
            is re-checking, and if a judge is on its way the answer will appear
            here without anyone pressing anything. It does not claim one is.
          */}
          {awaiting && !revision && !stoppedAsking && (
            <p className="note">
              這一頁每 3 秒再查一次；如果有評估正在排隊，結果會自己出現在這裡，不必重新整理。
            </p>
          )}
          {/*
            The promise ends when the polling does. 「結果會自己出現在這裡」 was
            true for the first minute and false for every hour after it, and an
            old run nobody ever evaluated spent that hour asking for a 404 every
            three seconds. What replaces it says which of the two happened and
            what the reader can do — not 「載入失敗」, because nothing failed.
          */}
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

      {evaluation.data && <EvaluationReport evaluation={evaluation.data} runStatus={runStatus} />}

      {/* No verdict for it to sit under, so it stands on its own here. With a
          verdict on screen it belongs immediately below it (design §2.5), which
          is where EvaluationReport renders it. */}
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
                // Merged rather than replaced: the advanced Trace's page
                // position lives on the same address.
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

      {/*
        回饋表單排在**建議之後**，不是報告的結尾。它以前長在 `EvaluationReport` 裡，
        於是判定出來之後往下捲，先遇到一個 textarea 加「有幫助／沒幫助」兩顆按鈕，
        再往下才是「改善建議」與那顆真正的主要動作「以已接受的 N 項建議建立新版本」
        ——**平台向使用者要回饋，排在使用者自己的下一步前面**，三顆按鈕在同一段捲動裡
        互相競爭。§4.5 剛量到這一頁在行長規則之後是 3070px，把 CTA 再往下推的代價是
        實測過的；§2.5 的歷史也記著同一個位置出過事（執行狀態一度落在「有幫助／
        沒幫助」按鈕與版本選單之間）。`disabled` 沿用同一個 `superseded_at` 判斷。
      */}
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

/**
 * The run's terminal state, worded as execution and never as a pass (ADR-025).
 * Design §2.5: the two axes are two rows and the task verdict is the first of
 * them — so this is a row directly under the verdict, not a paragraph that
 * happened to land after the whole report and beside the feedback form.
 */
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
      {/* Narrowed on the field, not on the boolean above it: `strict` is on
          now, and `Boolean(x)` does not tell the compiler `x` is a string. */}
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
              // Design §4.3: a finding carries its own category, severity, message
              // and evidence — a thing that can be believed or dismissed on its
              // own — so it is the card every other such list uses, and not a
              // fifth style. Same defect commit 90ade82 fixed for Test Case rows,
              // sitting directly under the .criterion cards it was unlike.
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

      <h3>評估本身的成本</h3>
      <p>
        {usd(evaluation.cost.evaluation_usd)}
        {evaluation.cost.source === "estimated" ? "（估算值）" : "（模型閘道實付）"}
      </p>
      <p className="note">
        {evaluation.cost.note}
        {" 這是平台判定花的錢，與 Run 自己花的錢分開列，不相加。"}
      </p>

      {/* Design §2.6 / checklist 5: the model, the two version strings and the
          timestamp are provenance, not the answer. Folded behind the same
          disclosure shape SkillDetail uses for version ids and hashes. */}
      <details>
        <summary>判定資訊（Judge 模型與版本）</summary>
        <ul className="note">
          <li>Judge 模型：{evaluation.judge_model || "未使用模型"}</li>
          <li>Judge prompt 版本：{evaluation.judge_prompt_version}</li>
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

/**
 * 設計 §2.13 去重 1, and `components/RunVerdict.tsx` is the shape it copies:
 * 「On a page of fifty rows a sentence per row is noise」. Four criteria all
 * judged by the model printed 判定來源：模型評估（不是確定事實） four times, and
 * from the second row on that sentence cannot make a reader decide anything
 * different — it is the LIST's fact, not the row's.
 *
 * So the source most of the list shares is stated once above it and the rows
 * that differ restate their own (`CriterionItem` below). Two conditions, both
 * load-bearing:
 *
 *  - **Counted from the data, never assumed.** 「模型評估」 hard-coded here would
 *    be a lie the first time a rule verdict arrives, which is the same defect
 *    §4.4 records `Home.tsx` shipping with `spec_validation`.
 *  - **Two rows minimum.** Hoisting a sentence that would only have been printed
 *    once ADDS text, and on a list of two disagreeing criteria there is nothing
 *    to deduplicate. Ties go to the first source seen (`Map` keeps insertion
 *    order), so the choice does not depend on how the object was built.
 *
 * Downgraded rows (04 丙-10) are excluded from the tally: they render the
 * platform's own sentence instead of `SOURCE_LABEL`, so counting them would let
 * a list where every verdict was thrown away hoist a source no row uses.
 */
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

/**
 * One acceptance criterion's verdict.
 *
 * The downgraded case (04 丙-10) is told apart on two channels, never on colour
 * alone (NFR-007): the badge says a different thing, and a sentence states who
 * decided. The border style differs as a third, and it is the only one a reader
 * could miss without losing the fact.
 */
function CriterionItem({
  criterion: c,
  listSource: shared,
}: {
  criterion: CriterionResult;
  listSource?: CriterionResult["source"];
}) {
  const downgraded = isEvidenceUnverifiable(c);

  return (
    <li className={`criterion criterion-${c.result}${downgraded ? " criterion-unverifiable" : ""}`}>
      <p>
        <span
          className={`badge badge-criterion-${c.result}${
            downgraded ? " badge-criterion-unverifiable" : ""
          }`}
        >
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
      {/* Untrusted when source is `model`: plain text, never markup. */}
      {c.reason && <p>{c.reason}</p>}
      <EvidenceList evidence={c.evidence} />
    </li>
  );
}

/**
 * ADR-026 決策 2. An expired citation keeps the excerpt taken when the verdict
 * was made and says the original is gone — not a blank, and not a pretence that
 * the trace event is still there.
 */
/**
 * ADR-043's two audit fields, on the screen.
 *
 * The ADR's own §影響 says it: an audit field that never reaches the screen is
 * not an audit field. G8 — two correct `failed` verdicts lost to a trailing
 * `}],` the model wrote inside a string value — would have been one glance at
 * 「正規化後比對」 instead of a regression report.
 *
 * Wording is client-side here, and that is deliberate rather than an oversight
 * of 設計系統 §4.4. An evaluation report is immutable and read long afterwards,
 * so a label written into it would freeze at storage time and every later
 * rewording would apply to new reports only — the same sentence appearing in two
 * forms across one list. The enum is the fact and it is stored; the sentence is
 * presentation and is not.
 *
 * `undefined` is its own case and does not fall through to 「已回驗」: reports
 * written before the field existed have no answer, and rendering silence as a
 * pass is exactly the failure ADR-043 was written about.
 */
/**
 * `undefined` is a fifth answer, not a missing fourth one — see the note above.
 * It is keyed here rather than left to a fallback expression so that the badge,
 * the sentence and the ordering all read it from one table.
 */
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

/**
 * 設計 §2.13「標籤與圖示」: the re-verification result is a STATE, so it is a
 * badge with a word in it and not a sentence per citation. Measured 2026-09-03,
 * the sentence版 was 232 characters — 19% of this page — because eight citations
 * each carried one of five fixed paragraphs.
 *
 * **The words carry the whole distinction** (§2.3: colour is the second channel,
 * and these five are three tints). 已逐字回驗 and 正規化後比對 are both 回驗過,
 * and they are NOT one word: ADR-043 exists because two correct `failed`
 * verdicts were lost to a trailing `}],`, and 「which of the two」 is exactly the
 * fact that would have shown it. 找不到 and 未回驗引文 are further apart still —
 * one is an accusation about a quote, the other says no quote was ever opened.
 *
 * **All five stay on the row.** 找不到／未回驗引文／回驗結果未記錄 are §2.9
 * absences and §2.10 第 10 項 is explicit: 缺席的「為什麼」可以折，缺席「是哪一型」
 * 不可折. What moves to the list is the explanation, never the type word.
 */
export const MATCH_WORD: Record<MatchKey, string> = {
  exact: "已逐字回驗",
  normalized: "正規化後比對",
  not_found: "找不到",
  not_checked: "未回驗引文",
  unrecorded: "回驗結果未記錄",
};

/**
 * §4.4 / §4.6.3: outline only, never a fill — a filled badge reads as an
 * endorsement. 找不到 is the one that stops a citation counting as evidence
 * (`--danger`, 這件事不通過); the two that were never verified take
 * `--accent-border`'s 未知／未檢查 tint; a citation that DID re-verify is 通過,
 * and §4.4 says a plain `.badge` is exactly that ("這是合法的").
 */
const MATCH_BADGE: Record<MatchKey, string> = {
  exact: "badge",
  normalized: "badge",
  not_found: "badge badge-danger",
  not_checked: "badge badge-unverified",
  unrecorded: "badge badge-unverified",
};

/** Print order for the legend. Stable, so two reports do not shuffle it. */
const MATCH_ORDER: MatchKey[] = ["exact", "normalized", "not_found", "not_checked", "unrecorded"];

function matchKey(e: EvidenceRef): MatchKey {
  return e.match ?? "unrecorded";
}

/**
 * 設計 §2.13 去重 1 — the explanations, stated once for the list that contains
 * the citations rather than once per citation.
 *
 * **Which list, and why not the citation's own `<ul>`.** A criterion usually
 * cites once, so a legend above each `.evidence-list` would print the same
 * sentence exactly as many times as the rows did and add eight badges on top of
 * it — deduplication that removes nothing. The list where the repetition
 * actually lives is 逐條驗收條件 / 這個 Run 的問題 / 改善建議: one legend above
 * each, covering every citation under it. That keeps §2.11(c)'s 「同一個區塊」 —
 * the block is the section, which is also how `Home.tsx` states its five
 * provenance markers once above the results list.
 *
 * **Counted, not written down.** Only the kinds actually present print, so a
 * report whose citations all verified exactly does not carry a paragraph about
 * a case it does not contain. The same page has already shipped the hard-coded
 * version of this mistake once (§0 的分類計數).
 */
function MatchLegend({ evidence }: { evidence: EvidenceRef[] }) {
  const kinds = MATCH_ORDER.filter((k) => evidence.some((e) => matchKey(e) === k));
  if (kinds.length === 0) return null;
  return (
    <ul className="note">
      {kinds.map((k) => (
        <li key={k}>
          <span className={MATCH_BADGE[k]}>{MATCH_WORD[k]}</span> {MATCH_NOTE[k]}
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
          {/* §2.11(c): the badge is the first signal and it is on the row, not
              in the legend — a reader who never reaches the legend still knows
              which of the five this citation is. */}
          <p className="note">
            <span className={MATCH_BADGE[matchKey(e)]}>{MATCH_WORD[matchKey(e)]}</span>{" "}
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
          {/* Design §2.6: the excerpt is the evidence; the event's uuid is an
              identifier and folds. A missing id says so rather than rendering
              as the blank §2.1 forbids — this line used to end in one. */}
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
    </div>
  );
}

/**
 * 02:EVAL-002 — the suggestions, their diffs, the per-item decision, and apply.
 *
 * The skill the new version goes under comes from GET /runs/{id}, not from the
 * page's URL: the apply call needs it, and a run page reachable without it could
 * only offer the action to readers who arrived by one particular link.
 */
function SuggestionsPanel({ runId }: { runId: string }) {
  const client = useQueryClient();
  const suggestions = useRunSuggestions(runId);
  const run = useRun(runId).data;
  const skillId = run?.skill_id;
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
  if (suggestions.isPending) return <Loading what="改善建議" />;
  if (notFound) return null;
  if (suggestions.error) {
    return <ReadFailure error={suggestions.error} what="改善建議" />;
  }

  const accepted = suggestions.data.suggestions.filter((s) => s.decision === "accepted");

  return (
    // Child of 任務判定 by data (keyed on evaluation_id), by gating
    // (status === "completed") and by DOM — so h3, not a second h2 pretending to
    // be its sibling (checklist 6). Demoting the heading is the smaller change
    // than lifting the section out, and the nesting it states is the true one.
    <section>
      <h3>改善建議</h3>
      {suggestions.data.suggestions.length === 0 ? (
        <p>這份評估沒有產生改善建議。</p>
      ) : (
        <>
          {/* 設計 §2.13 去重 1. The qualification is C 類 (§2.11(c): what the
              number does NOT cover) so not a word of it may go — but it does
              not change per suggestion, so it is the list's sentence and not
              the row's. Same move as 判定來源 above. */}
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

      {message && <p role="alert">{message}</p>}
      {applied && <AppliedResult result={applied} testCaseId={run?.test_case_id} />}
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
  if (diff.isPending) return <Loading what="差異" />;
  if (diff.error) return <ReadFailure error={diff.error} what="差異" />;

  return (
    <div>
      {/* 設計 §4.6.3（ADR-064）：這一則是阻斷——「無法套用」說的是採用建議這件事
          不會發生，不是它會發生但少了一點。同頁另外五則（評估進行中、未評估、
          歷史判定、評估未完成、材料不完整）都是降級或狀態，維持原樣。 */}
      {!diff.data.applicable && (
        <p className="notice notice-danger">
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

/**
 * 02:EVAL-003 第 1 條. The step that used to be missing: the new version's id
 * handed to the preflight screen, instead of the user copying it off the skill
 * page and editing the address bar.
 *
 * The ids come from the apply response (`skill_id`/`version_id`, both required
 * by contract) and from GET /runs/{id} — never invented here. `duplicate: true`
 * still links: the id points at the existing version with the same content,
 * which is as runnable as a freshly built one. No `test_case_id` means the
 * draft this run was frozen from no longer resolves, so the link is dropped
 * rather than pointed at something that would 404 (ADR-003, same rule as
 * `RerunCell`).
 *
 * It is a link, not a run: the destination is the permission screen and the
 * user still confirms the summary hash there (TEST-009).
 */
function AppliedResult({
  result,
  testCaseId,
}: {
  result: VersionFromSuggestions;
  testCaseId?: string;
}) {
  return (
    // Design §4.3: notice is a standing platform condition. This is the result
    // of the action the user just took, which is what the rest of the app says
    // with role="status" — so it takes that surface, not the platform's.
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
