import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { ApiError, isLastingReadFailure } from "../../../core/api/client";
import type { UseQueryResult } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { continuationRoomOf, useRun, useRunDetailFollowsTrace } from "../runs.service";
import { InFlight } from "./components/InFlight";
import { useTrace, type TraceSummary } from "../trace.service";
import { EvaluationPanel } from "../evaluation/EvaluationPanel";
import { CANCELLABLE, CancelRunControl } from "./components/CancelRunControl";
import { RunArtifacts } from "./components/RunArtifacts";
import { GeneralMode } from "./components/GeneralMode";
import { AnswerQuestions } from "./components/AnswerQuestions";
import { AdvancedMode } from "./components/AdvancedMode";
import { SkillWorkspaceNav } from "../../skill";
import "./RunTrace.page.css";

function RunSourceContext({
  skillId,
  versionId,
  testCaseId,
}: {
  skillId: string;
  versionId: string;
  testCaseId?: string;
}) {
  return (
    <nav className="download-item run-source-context" aria-label="這次試跑的來源">
      <strong>這次試跑的來源</strong>
      <ul className="chip-row">
        <li>
          <Link to="/skills/$skillId" params={{ skillId }}>
            來源小工具
          </Link>
        </li>
        <li>
          <Link to="/skills/$skillId/versions/$versionId" params={{ skillId, versionId }}>
            來源 Version
          </Link>
        </li>
        {testCaseId && (
          <li>
            <Link
              to="/lab/test-cases/$testCaseId"
              params={{ testCaseId }}
              search={{ version: versionId }}
            >
              來源測試題
            </Link>
          </li>
        )}
      </ul>
    </nav>
  );
}

function RunWorkspaceRail({ runId, status }: { runId: string; status?: string }) {
  return (
    <aside className="run-workspace-rail" aria-label="試跑紀錄操作與區段導覽">
      <section>
        <h2>這次試跑</h2>
        <nav aria-label="試跑結果導覽">
          <ul className="run-section-nav">
            <li>
              <a href="#run-decision">任務判定</a>
            </li>
            <li>
              <a href="#run-trace">執行紀錄</a>
            </li>
            <li>
              <a href="#run-artifacts">產出</a>
            </li>
          </ul>
        </nav>
        <p>
          <Link to="/runs/$runId/compare" params={{ runId }}>
            與另一個試跑比較
          </Link>
        </p>
        <CancelRunControl runId={runId} status={status} />
      </section>
    </aside>
  );
}

function retainedQuestions(runId: string, query: UseQueryResult<TraceSummary>) {
  const data = query.data;
  if (
    data?.run_id !== runId ||
    data.status !== "succeeded" ||
    unauthenticated(query.error) ||
    isLastingReadFailure(query.error)
  )
    return undefined;
  return data.questions;
}

export function RunTrace() {
  const { runId } = useParams({ from: "/runs/$runId" });
  const { events: linkedEvents } = useSearch({ strict: false }) as { events?: string };
  const [mode, setMode] = useState<"general" | "advanced">(linkedEvents ? "advanced" : "general");
  const run = useRun(runId);
  const general = useTrace(runId, "general");
  const readableRun = run.error ? undefined : run.data;
  const readableTrace = general.error ? undefined : general.data;
  const runGone = run.error instanceof ApiError && run.error.status === 404 && !run.data;
  useRunDetailFollowsTrace(run, readableTrace?.status);

  return (
    <article className="run-workspace">
      <header className="run-workspace-header">
        <p className="run-eyebrow">驗證與執行</p>
        <h1 id="run-heading" tabIndex={-1}>
          試跑結果
        </h1>
        <details>
          <summary>試跑識別碼</summary>
          <p className="run-identity" data-role="evidence">
            <strong>試跑紀錄 ID：</strong>
            <code>{runId}</code>
          </p>
        </details>
        <ReadFailure
          error={run.error}
          what="這次試跑"
          retry={runGone ? undefined : () => void run.refetch()}
          retrying={run.isFetching}
          focusTarget="run-heading"
        >
          {runGone && (
            <p role="alert">
              找不到這次試跑，或它不屬於你的工作區，重新讀取也不會出現。
              <Link to="/workspace/runs">回到試跑紀錄</Link>
            </p>
          )}
        </ReadFailure>
        {readableRun && (
          <>
            <SkillWorkspaceNav
              skillId={readableRun.skill_id}
              versionId={readableRun.skill_version_id}
              testCaseId={readableRun.test_case_id}
            />
            <RunSourceContext
              skillId={readableRun.skill_id}
              versionId={readableRun.skill_version_id}
              testCaseId={readableRun.test_case_id}
            />
          </>
        )}
      </header>

      {!runGone && (
        <RunWorkspaceBody
          runId={runId}
          mode={mode}
          setMode={setMode}
          readableRun={readableRun}
          general={general}
        />
      )}
    </article>
  );
}

function RunWorkspaceBody({
  runId,
  mode,
  setMode,
  readableRun,
  general,
}: {
  runId: string;
  mode: "general" | "advanced";
  setMode: (mode: "general" | "advanced") => void;
  readableRun: ReturnType<typeof useRun>["data"];
  general: UseQueryResult<TraceSummary>;
}) {
  const readableTrace = general.error ? undefined : general.data;
  const questions = retainedQuestions(runId, general);
  return (
    <>
      <section className="run-workspace-panel run-decision" id="run-decision">
        <EvaluationPanel runId={runId} runStatus={readableTrace?.status} />
        {readableTrace && <InFlight summary={readableTrace} />}
      </section>

      <div className="run-workspace-layout">
        <RunWorkspaceRail runId={runId} status={readableTrace?.status} />

        <div className="run-workspace-main">
          <section className="run-workspace-panel" id="run-trace">
            <p className="run-eyebrow">執行證據</p>
            <h2 id="run-trace-heading" tabIndex={-1}>
              執行紀錄
            </h2>
            {questions?.length ? (
              <AnswerQuestions
                key={runId}
                runId={runId}
                questions={questions}
                room={continuationRoomOf(readableRun)}
                readConfirmed={general.isSuccess && general.fetchStatus === "idle"}
                checking={general.fetchStatus !== "idle"}
                onRetryRead={() => void general.refetch()}
              />
            ) : null}
            <p className="note" data-role="teaching">
              一般模式是摘要，進階模式是這次試跑的原始事件（已遮罩）——同一份紀錄的兩種詳細度，
              只影響下面這一節。
            </p>
            <div className="run-mode-switch" role="group" aria-label="執行紀錄的詳細度">
              <button
                type="button"
                aria-pressed={mode === "general"}
                onClick={() => setMode("general")}
              >
                一般模式
              </button>
              <button
                type="button"
                aria-pressed={mode === "advanced"}
                onClick={() => setMode("advanced")}
              >
                進階模式
              </button>
            </div>
            {mode === "general" ? (
              <GeneralMode runId={runId} general={general} />
            ) : (
              <AdvancedMode
                key={runId}
                runId={runId}
                active={Boolean(readableTrace?.status && CANCELLABLE.has(readableTrace.status))}
              />
            )}
          </section>

          <section className="run-workspace-panel" id="run-artifacts">
            <RunArtifacts runId={runId} />
          </section>
        </div>
      </div>
    </>
  );
}
